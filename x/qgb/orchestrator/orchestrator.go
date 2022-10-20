package orchestrator

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"strconv"
	"sync"
	"time"

	"github.com/celestiaorg/celestia-app/x/qgb/orchestrator/store"

	"github.com/celestiaorg/celestia-app/x/qgb/orchestrator/api"
	"github.com/celestiaorg/celestia-app/x/qgb/orchestrator/evm"
	"github.com/celestiaorg/celestia-app/x/qgb/orchestrator/utils"

	paytypes "github.com/celestiaorg/celestia-app/x/payment/types"
	"github.com/celestiaorg/celestia-app/x/qgb/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdktypestx "github.com/cosmos/cosmos-sdk/types/tx"
	ethcmn "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pkg/errors"
	tmlog "github.com/tendermint/tendermint/libs/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var _ I = &Orchestrator{}

type I interface {
	Start(ctx context.Context, signalChan chan struct{})
	StartNewEventsListener(ctx context.Context, queue chan<- uint64, lastNonce uint64, signalChan <-chan struct{}) error
	EnqueueMissingEvents(ctx context.Context, queue chan<- uint64, lastNonce uint64, signalChan <-chan struct{}) error
	ProcessNonces(ctx context.Context, noncesQueue <-chan uint64, requeueChan chan<- uint64, signalChan chan<- struct{}) error
	Process(ctx context.Context, nonce uint64, requeueChan chan<- uint64) error
	ProcessValsetEvent(ctx context.Context, valset types.Valset) error
	ProcessDataCommitmentEvent(ctx context.Context, dc types.DataCommitment) error
}

type Orchestrator struct {
	Logger tmlog.Logger // maybe use a more general interface

	EvmPrivateKey  ecdsa.PrivateKey // TODO unexport these members
	Signer         *paytypes.KeyringSigner
	OrchEthAddress ethcmn.Address
	OrchAccAddress sdk.AccAddress

	tmQuerier   api.TmQuerierI // TODO no need to export all of these
	qgbQuerier  api.QGBQuerierI
	Broadcaster BroadcasterI
	Retrier     RetrierI
	startHeight int64
}

// TODO wait for ingestion to finish before starting to sign
// Write issue concerning how to do both at the same time
// TODO add some milestone mechanism to know how much is ingested
func NewOrchestrator(
	logger tmlog.Logger,
	stateQuerier api.TmQuerierI,
	storeQuerier api.QGBQuerierI,
	broadcaster BroadcasterI,
	retrier RetrierI,
	signer *paytypes.KeyringSigner,
	evmPrivateKey ecdsa.PrivateKey,
	startHeight int64,
) (*Orchestrator, error) {
	orchEthAddr := crypto.PubkeyToAddress(evmPrivateKey.PublicKey)

	orchAccAddr, err := signer.GetSignerInfo().GetAddress()
	if err != nil {
		return nil, err
	}

	return &Orchestrator{
		Logger:         logger,
		Signer:         signer,
		EvmPrivateKey:  evmPrivateKey,
		OrchEthAddress: orchEthAddr,
		tmQuerier:      stateQuerier,
		qgbQuerier:     storeQuerier,
		Broadcaster:    broadcaster,
		Retrier:        retrier,
		OrchAccAddress: orchAccAddr,
		startHeight:    startHeight,
	}, nil
}

func (orch Orchestrator) Start(ctx context.Context, signalChan chan struct{}) {
	// contains the nonces that will be signed by the orchestrator.
	noncesQueue := make(chan uint64, 100)
	defer close(noncesQueue)

	// used to send a signal when the nonces processor wants to notify the nonces enqueuing services to stop.
	// signalChan := make(chan struct{})

	withCancel, cancel := context.WithCancel(ctx)

	lastNonce, err := orch.getLastAttestationNonce(ctx, signalChan)
	if err != nil {
		cancel()
		return
	}

	wg := &sync.WaitGroup{}

	// FIXME the orchestrator is enqueuing twice nonces when listening for new events
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := orch.StartNewEventsListener(withCancel, noncesQueue, lastNonce, signalChan)
		if err != nil {
			orch.Logger.Error("orch: error listening to new attestations", "err", err)
			cancel()
		}
		orch.Logger.Info("orch: stopping listening to new attestations")
	}()

	requeueChan := make(chan uint64, 100)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case nonce := <-requeueChan:
				noncesQueue <- nonce
			case <-signalChan:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := orch.ProcessNonces(withCancel, noncesQueue, requeueChan, signalChan)
		if err != nil {
			orch.Logger.Error("orch: error processing attestations", "err", err)
			cancel()
		}
		orch.Logger.Error("orch: stopping processing attestations")
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := orch.EnqueueMissingEvents(withCancel, noncesQueue, lastNonce, signalChan)
		if err != nil {
			orch.Logger.Error("orch: error enqueing missing attestations", "err", err)
			cancel()
		}
		orch.Logger.Error("orch: stopping enqueing missing attestations")
	}()

	// FIXME should we add  another go routine that keep checking if all the attestations
	// were signed every 10min for example?

	wg.Wait()
}

// TODO what the fuck is wrong with you!
func (orch Orchestrator) StartNewEventsListener(
	ctx context.Context,
	queue chan<- uint64,
	currentNonce uint64,
	signalChan <-chan struct{},
) error {
	orch.Logger.Info("orch: listening for new attestation nonces...")
	ticker := time.NewTicker(1 * time.Second)
	for {
		select {
		case <-signalChan:
			return nil
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			lastNonce, err := orch.getLastAttestationNonce(ctx, signalChan)
			if err != nil {
				return err
			}
			if currentNonce == lastNonce {
				continue
			}
			// the for loop is not to miss attestations if stuck with a full nonces queue.
			// or, too many changes happened.
			for n := currentNonce; n <= lastNonce; n++ {
				select {
				case <-signalChan:
					return nil
				case queue <- uint64(n):
					orch.Logger.Debug("orch: enqueueing new attestation nonce", "nonce", n)
				}
			}
			currentNonce = lastNonce
		}
	}
}

// TODO update missing events and new events names
func (orch Orchestrator) EnqueueMissingEvents(
	ctx context.Context,
	queue chan<- uint64,
	latestNonce uint64,
	signalChan <-chan struct{},
) error {
	// TODO use either lastNonce or latestNonce across all places
	lastUnbondingAttestationNonce, err := orch.getLastUnbondingAttestationNonce(ctx, signalChan) // TODO should wait and see
	if err != nil {
		return err
	}

	orch.Logger.Info("orch: syncing missing nonces", "latest_nonce", latestNonce, "last_unbonding_attestation_nonce", lastUnbondingAttestationNonce)

	for i := lastUnbondingAttestationNonce; i <= latestNonce; i++ {
		select {
		case <-signalChan:
			return nil
		case <-ctx.Done():
			return nil
		default:
			// TODO add a const called LogsFrequency
			if i%100 == 0 {
				orch.Logger.Debug("orch: enqueueing missing attestation nonce", "start_nonce", latestNonce-i, "end_nonce", lastUnbondingAttestationNonce)
			}
			select {
			case <-signalChan:
				return nil
			case queue <- latestNonce - i:
			}
		}
	}
	orch.Logger.Info("orch: finished syncing missing nonces", "latest_nonce", latestNonce, "last_unbonding_attestation_nonce", lastUnbondingAttestationNonce)
	return nil
}

func (orch Orchestrator) ProcessNonces(
	ctx context.Context,
	noncesQueue <-chan uint64,
	requeueChan chan<- uint64,
	signalChan chan<- struct{},
) error {
	for {
		select { // TODO All for loops and sleeps should select on error channels
		case <-ctx.Done():
			// close(signalChan) // TODO investigate if this is needed
			return nil
		case nonce := <-noncesQueue:
			orch.Logger.Debug("orch: processing nonce", "nonce", nonce)
			if err := orch.Process(ctx, nonce, requeueChan); err != nil {
				orch.Logger.Error("orch: failed to process nonce, retrying", "nonce", nonce, "err", err)
				if err := orch.Retrier.Retry(ctx, nonce, requeueChan, orch.Process); err != nil {
					close(signalChan)
					return err
				}
			}
		}
	}
}

func (orch Orchestrator) Process(ctx context.Context, nonce uint64, requeueChan chan<- uint64) error {
	// at this level, the LastUnbondingAttestationNonce will not have the default value
	// as the orchestrator waits in the beginning for this value to be changed before starting.
	lastUnbondingNonce, err := orch.qgbQuerier.QueryLastUnbondingAttestationNonce(ctx)
	if err != nil {
		return err
	}
	// this check in case the unbonding period changed after enqueuing the attestations nonces
	// not to sign unnecessary attestations.
	if nonce < lastUnbondingNonce {
		orch.Logger.Debug("orch: attestation nonce older than last unbonding height nonce. not signing", "nonce", nonce, "last_unbonding_nonce", lastUnbondingNonce)
		return nil
	}
	att, err := orch.qgbQuerier.QueryAttestationByNonce(ctx, nonce)
	if err != nil {
		return err
	}
	if att == nil {
		//  TODO requeue
		requeueChan <- nonce
		return nil
	}
	// TODO uncomment this. Also, not check unless the ingestor finished ingesting all stuff
	// check if the validator is part of the needed valset
	var previousValset *types.Valset
	if att.GetNonce() == 1 {
		// if nonce == 1, then, the current valset should sign the confirm.
		// In fact, the first nonce should never be signed. Because, the first attestation, in the case
		// where the `earliest` flag is specified when deploying the contract, will be relayed as part of
		// the deployment of the QGB contract.
		// It will be signed temporarily for now.
		previousValset, err = orch.qgbQuerier.QueryValsetByNonce(ctx, att.GetNonce())
		if err != nil {
			return err
		}
	} else {
		previousValset, err = orch.qgbQuerier.QueryLastValsetBeforeNonce(ctx, att.GetNonce())
		if err != nil {
			return err
		}
	}
	if !ValidatorPartOfValset(previousValset.Members, orch.OrchEthAddress.Hex()) {
		// no need to sign if the orchestrator is not part of the validator set that needs to sign the attestation
		orch.Logger.Debug("orch: validator not part of valset. won't sign", "nonce", nonce)
		return nil
	}
	switch att.Type() {
	case types.ValsetRequestType:
		vs, ok := att.(*types.Valset)
		if !ok {
			return errors.Wrap(types.ErrAttestationNotValsetRequest, strconv.FormatUint(nonce, 10))
		}
		// TODO recheck this milestone implementation if it does what it's supposed to
		if int64(vs.Height) > orch.qgbQuerier.GetStorageHeightsMilestone() &&
			int64(vs.Height) <= orch.startHeight {
			requeueChan <- nonce
			return nil
		}
		resp, err := orch.qgbQuerier.QueryValsetConfirmByOrchestratorAddress(ctx, nonce, orch.OrchAccAddress.String())
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("valset %d", nonce))
		}
		if !utils.IsEmptyMsgValsetConfirm(resp) {
			orch.Logger.Debug("orch: already signed valset", "nonce", nonce, "signature", resp.Signature)
			return nil
		}
		err = orch.ProcessValsetEvent(ctx, *vs)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("valset %d", nonce))
		}
		return nil

	case types.DataCommitmentRequestType:
		dc, ok := att.(*types.DataCommitment)
		if !ok {
			return errors.Wrap(types.ErrAttestationNotDataCommitmentRequest, strconv.FormatUint(nonce, 10))
		}
		// TODO recheck this milestone implementation if it does what it's supposed to
		if int64(dc.EndBlock) > orch.qgbQuerier.GetStorageHeightsMilestone() &&
			int64(dc.EndBlock) <= orch.startHeight {
			requeueChan <- nonce
			return nil
		}
		resp, err := orch.qgbQuerier.QueryDataCommitmentConfirmByOrchestratorAddress(
			ctx,
			dc.Nonce,
			orch.OrchAccAddress.String(),
		)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("data commitment %d", nonce))
		}
		if !utils.IsEmptyMsgDataCommitmentConfirm(resp) {
			orch.Logger.Debug("orch: already signed data commitment", "nonce", nonce, "begin_block", resp.BeginBlock, "end_block", resp.EndBlock, "commitment", resp.Commitment, "signature", resp.Signature)
			return nil
		}
		err = orch.ProcessDataCommitmentEvent(ctx, *dc)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("data commitment %d", nonce))
		}
		return nil

	default:
		return errors.Wrap(ErrUnknownAttestationType, strconv.FormatUint(nonce, 10))
	}
}

// Add from current shitty unbonding period
// keep going up and down

func (orch Orchestrator) ProcessValsetEvent(ctx context.Context, valset types.Valset) error {
	signBytes, err := valset.SignBytes(types.BridgeID)
	if err != nil {
		return err
	}
	signature, err := evm.NewEthereumSignature(signBytes.Bytes(), &orch.EvmPrivateKey)
	if err != nil {
		return err
	}

	// create and send the valset hash
	msg := utils.NewMsgValsetConfirm(
		valset.Nonce,
		orch.OrchEthAddress,
		orch.OrchAccAddress,
		ethcmn.Bytes2Hex(signature),
	)
	hash, err := orch.Broadcaster.BroadcastTx(ctx, msg)
	if err != nil {
		return err
	}
	orch.Logger.Info("orch: signed Valset", "nonce", msg.Nonce, "tx_hash", hash)
	return nil
}

func (orch Orchestrator) ProcessDataCommitmentEvent(
	ctx context.Context,
	dc types.DataCommitment,
) error {
	commitment, err := orch.tmQuerier.QueryCommitment(
		ctx,
		dc.BeginBlock,
		dc.EndBlock,
	)
	if err != nil {
		return err
	}
	dataRootHash := utils.DataCommitmentTupleRootSignBytes(types.BridgeID, big.NewInt(int64(dc.Nonce)), commitment)
	dcSig, err := evm.NewEthereumSignature(dataRootHash.Bytes(), &orch.EvmPrivateKey)
	if err != nil {
		return err
	}

	msg := utils.NewMsgDataCommitmentConfirm(
		commitment.String(),
		ethcmn.Bytes2Hex(dcSig),
		orch.OrchAccAddress,
		orch.OrchEthAddress,
		dc.BeginBlock,
		dc.EndBlock,
		dc.Nonce,
	)
	hash, err := orch.Broadcaster.BroadcastTx(ctx, msg)
	if err != nil {
		return err
	}
	orch.Logger.Info("orch: signed commitment", "nonce", msg.Nonce, "begin_block", msg.BeginBlock, "end_block", msg.EndBlock, "commitment", commitment, "tx_hash", hash)
	return nil
}

// comment please
func (orch Orchestrator) getLastAttestationNonce(ctx context.Context, signalChan <-chan struct{}) (uint64, error) {
	ticker := time.NewTicker(1 * time.Second)
	for {
		select {
		case <-signalChan:
			return 0, nil // TODO check if nil is what needs to be returned
		case <-ctx.Done():
			return 0, nil // TODO check if nil is what needs to be returned
		case <-ticker.C:
			lastNonce, err := orch.qgbQuerier.QueryLatestAttestationNonce(ctx)
			if err != nil {
				return 0, err
			}
			if lastNonce != store.DefaultLastAttestationNonce {
				return lastNonce, nil
			}
		}
	}
}

// getLastUnbondingAttestationNonce
func (orch Orchestrator) getLastUnbondingAttestationNonce(ctx context.Context, signalChan <-chan struct{}) (uint64, error) {
	ticker := time.NewTicker(1 * time.Second)
	for {
		select {
		case <-signalChan:
			return 0, nil // TODO check if nil is what needs to be returned
		case <-ctx.Done():
			return 0, nil // TODO check if nil is what needs to be returned
		case <-ticker.C:
			lastNonce, err := orch.qgbQuerier.QueryLastUnbondingAttestationNonce(ctx)
			if err != nil {
				return 0, err
			}
			if lastNonce != store.DefaultLastUnbondingHeightAttestationNonce {
				return lastNonce, nil
			}
		}
	}
}

const DEFAULTCELESTIAGASLIMIT = 100000

var _ BroadcasterI = &Broadcaster{}

type BroadcasterI interface {
	BroadcastTx(ctx context.Context, msg sdk.Msg) (string, error)
}

type Broadcaster struct {
	mutex            *sync.Mutex
	signer           *paytypes.KeyringSigner
	qgbGrpc          *grpc.ClientConn
	celestiaGasLimit uint64
}

func NewBroadcaster(qgbGrpcAddr string, signer *paytypes.KeyringSigner, celestiaGasLimit uint64) (*Broadcaster, error) {
	qgbGrpc, err := grpc.Dial(qgbGrpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	return &Broadcaster{
		mutex:            &sync.Mutex{}, // investigate if this is needed
		signer:           signer,
		qgbGrpc:          qgbGrpc,
		celestiaGasLimit: celestiaGasLimit,
	}, nil
}

func (bc *Broadcaster) BroadcastTx(ctx context.Context, msg sdk.Msg) (string, error) {
	bc.mutex.Lock()
	defer bc.mutex.Unlock()
	err := bc.signer.QueryAccountNumber(ctx, bc.qgbGrpc)
	if err != nil {
		return "", err
	}

	builder := bc.signer.NewTxBuilder()
	builder.SetGasLimit(bc.celestiaGasLimit)
	// TODO: update this api
	// via https://github.com/celestiaorg/celestia-app/pull/187/commits/37f96d9af30011736a3e6048bbb35bad6f5b795c
	tx, err := bc.signer.BuildSignedTx(builder, msg)
	if err != nil {
		return "", err
	}

	rawTx, err := bc.signer.EncodeTx(tx)
	if err != nil {
		return "", err
	}

	// FIXME sdktypestx.BroadcastMode_BROADCAST_MODE_BLOCK waits for a block to be minted containing
	// the transaction to continue. This makes the orchestrator slow to catchup.
	// It would be better to just send the transaction. Then, another job would keep an eye
	// if the transaction was included. If not, retry it. But this would mean we should increment ourselves
	// the sequence number after each broadcasted transaction.
	// We can also use BroadcastMode_BROADCAST_MODE_SYNC but it will also fail due to a non incremented
	// sequence number.

	// TODO  check if we can move this outside of the paytypes
	resp, err := paytypes.BroadcastTx(ctx, bc.qgbGrpc, sdktypestx.BroadcastMode_BROADCAST_MODE_BLOCK, rawTx)
	if err != nil {
		return "", err
	}

	if resp.TxResponse.Code != 0 {
		return "", errors.Wrap(ErrFailedBroadcast, resp.TxResponse.RawLog)
	}

	return resp.TxResponse.TxHash, nil
}

var _ RetrierI = &Retrier{}

type Retrier struct {
	logger        tmlog.Logger
	retriesNumber int
}

func NewRetrier(logger tmlog.Logger, retriesNumber int) *Retrier {
	return &Retrier{
		logger:        logger,
		retriesNumber: retriesNumber,
	}
}

type RetrierI interface {
	Retry(ctx context.Context, nonce uint64, queueChan chan<- uint64, retryMethod func(context.Context, uint64, chan<- uint64) error) error
	RetryThenFail(ctx context.Context, nonce uint64, requeueChan chan<- uint64, retryMethod func(context.Context, uint64, chan<- uint64) error)
}

func (r Retrier) Retry(ctx context.Context, nonce uint64, requeueChan chan<- uint64, retryMethod func(context.Context, uint64, chan<- uint64) error) error {
	var err error
	for i := 0; i <= r.retriesNumber; i++ {
		// We can implement some exponential backoff in here
		select {
		case <-ctx.Done():
			return nil
		default:
			time.Sleep(10 * time.Second)
			r.logger.Info("retrying", "nonce", nonce, "retry_number", i, "retries_left", r.retriesNumber-i)
			err = retryMethod(ctx, nonce, requeueChan)
			if err == nil {
				r.logger.Info("nonce processing succeeded", "nonce", nonce, "retries_number", i)
				return nil
			}
			r.logger.Error("failed to process nonce", "nonce", nonce, "retry", i, "err", err)
		}
	}
	return err
}

func (r Retrier) RetryThenFail(ctx context.Context, nonce uint64, requeueChan chan<- uint64, retryMethod func(context.Context, uint64, chan<- uint64) error) {
	err := r.Retry(ctx, nonce, requeueChan, retryMethod)
	if err != nil {
		panic(err)
	}
}

func ValidatorPartOfValset(members []types.BridgeValidator, ethAddr string) bool {
	for _, val := range members {
		if val.EthereumAddress == ethAddr {
			return true
		}
	}
	return false
}
