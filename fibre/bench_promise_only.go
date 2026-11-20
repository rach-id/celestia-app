package fibre

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"sync"

	fibregrpc "github.com/celestiaorg/celestia-app/v11/fibre/internal/grpc"
	"github.com/celestiaorg/celestia-app/v11/x/fibre/types"
	"github.com/celestiaorg/go-square/v4/share"
	core "github.com/cometbft/cometbft/types"
	txsigning "github.com/cosmos/cosmos-sdk/types/tx/signing"
)

// BenchPromiseOnly makes UploadShard sign a verified promise that carries no
// shard, skipping assignment, shard verification and storage. Bench only.
var BenchPromiseOnly = os.Getenv("CELESTIA_BENCH_FIBRE_PROMISE_ONLY") == "1"

// UploadPromiseOnly sends a promise for dataSize bytes with a random commitment
// and no shard to every validator and collects their signatures. Bench only:
// validators must run with CELESTIA_BENCH_FIBRE_PROMISE_ONLY=1.
func (c *Client) UploadPromiseOnly(ctx context.Context, ns share.Namespace, dataSize int, opts ...UploadOption) (SignedPaymentPromise, error) {
	if !c.started.Load() {
		return SignedPaymentPromise{}, errClientNotStarted
	}
	if c.closed.Load() {
		return SignedPaymentPromise{}, ErrClientClosed
	}
	if c.keyring == nil {
		return SignedPaymentPromise{}, ErrNoKeyring
	}
	opt := uploadOptions{keyName: c.Config.DefaultKeyName}
	for _, o := range opts {
		o(&opt)
	}

	valSet, err := c.validatorSet(ctx, 0)
	if err != nil {
		return SignedPaymentPromise{}, fmt.Errorf("fibre: getting validator set: %w", err)
	}
	c.warm(valSet)

	signerKey, err := c.signerKey(opt.keyName)
	if err != nil {
		return SignedPaymentPromise{}, err
	}
	cfg := DefaultBlobConfigV0()
	promise := &PaymentPromise{
		ChainID:           c.state.ChainID(),
		Height:            valSet.Height,
		Namespace:         ns,
		UploadSize:        uint32(cfg.UploadSize(dataSize)),
		BlobVersion:       uint32(cfg.BlobVersion),
		CreationTimestamp: c.clock.Now().UTC(),
		SignerKey:         signerKey,
	}
	if _, err := rand.Read(promise.Commitment[:]); err != nil {
		return SignedPaymentPromise{}, err
	}
	signBytes, err := promise.SignBytes()
	if err != nil {
		return SignedPaymentPromise{}, err
	}
	if promise.Signature, _, err = c.keyring.Sign(opt.keyName, signBytes, txsigning.SignMode_SIGN_MODE_DIRECT); err != nil {
		return SignedPaymentPromise{}, fmt.Errorf("signing payment promise: %w", err)
	}
	pb, err := promise.ToProto()
	if err != nil {
		return SignedPaymentPromise{}, err
	}
	req := &types.UploadShardRequest{Promise: pb}
	sigSet := valSet.NewSignatureSet(c.Config.SafetyThreshold, signBytes)

	quorum, all := make(chan struct{}), make(chan struct{})
	var once sync.Once
	var wg sync.WaitGroup
	for _, val := range valSet.Validators {
		wg.Add(1)
		c.closeWg.Add(1)
		go func(val *core.Validator) {
			defer c.closeWg.Done()
			defer wg.Done()
			var resp *types.UploadShardResponse
			err := c.clientCache.Request(ctx, val, func(client fibregrpc.Client) error {
				rpcCtx, cancel := context.WithTimeout(ctx, c.Config.RPCTimeout)
				defer cancel()
				var err error
				resp, err = client.UploadShard(rpcCtx, req)
				return err
			})
			if err != nil {
				c.log.WarnContext(ctx, "promise-only upload failed", "validator", val.Address.String(), "error", err)
				return
			}
			if ok, err := sigSet.Add(val, resp.ValidatorSignature); err == nil && ok {
				once.Do(func() { close(quorum) })
			}
		}(val)
	}
	go func() { wg.Wait(); close(all) }()
	select {
	case <-quorum:
	case <-all:
	case <-ctx.Done():
		return SignedPaymentPromise{}, ctx.Err()
	}
	sigs, err := sigSet.Signatures()
	if err != nil {
		return SignedPaymentPromise{}, err
	}
	return SignedPaymentPromise{PaymentPromise: promise, ValidatorSignatures: sigs}, nil
}
