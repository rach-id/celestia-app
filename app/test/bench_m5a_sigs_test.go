package app_test

import (
	"crypto/rand"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/celestiaorg/celestia-app/v11/app"
	"github.com/celestiaorg/celestia-app/v11/app/encoding"
	"github.com/celestiaorg/celestia-app/v11/fibre"
	"github.com/celestiaorg/celestia-app/v11/fibre/validator"
	"github.com/celestiaorg/celestia-app/v11/pkg/user"
	testutil "github.com/celestiaorg/celestia-app/v11/test/util"
	"github.com/celestiaorg/celestia-app/v11/test/util/blobfactory"
	"github.com/celestiaorg/celestia-app/v11/test/util/testfactory"
	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cometbft/cometbft/crypto/ed25519"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	core "github.com/cometbft/cometbft/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	"github.com/stretchr/testify/require"
)

// TestM5aPFFSignatureCost is bench only (M5A_SIG=1): it times CheckTx, ProcessProposal,
// FinalizeBlock and Commit for blocks of PFFs carrying a quorum certificate from a
// 1-validator set and from M5A_POWERS (fleet validator_power.csv), one JSON line per config.
func TestM5aPFFSignatureCost(t *testing.T) {
	if os.Getenv("M5A_SIG") != "1" {
		t.Skip("bench only")
	}
	m, _ := strconv.Atoi(os.Getenv("M5A_TXS"))
	if m == 0 {
		m = 600
	}
	rounds := 3
	configs := []struct {
		name   string
		powers []int64
	}{{"vals=1", []int64{1}}, {"vals=100", m5aPowers(t)}}
	for _, c := range configs {
		t.Run(c.name, func(t *testing.T) { m5aRun(t, c.name, c.powers, m, rounds) })
	}
}

func m5aPowers(t *testing.T) []int64 {
	f, err := os.Open(os.Getenv("M5A_POWERS"))
	require.NoError(t, err)
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	require.NoError(t, err)
	var powers []int64
	for _, r := range rows[1:] {
		p, err := strconv.ParseInt(r[3], 10, 64)
		require.NoError(t, err)
		powers = append(powers, p)
	}
	return powers
}

func m5aRun(t *testing.T, name string, powers []int64, m, rounds int) {
	enc := encoding.MakeConfig(app.ModuleEncodingRegisters...)
	keys := make([]ed25519.PrivKey, len(powers))
	vals := make([]*core.Validator, len(powers))
	byAddr := map[string]ed25519.PrivKey{}
	for i, p := range powers {
		keys[i] = ed25519.GenPrivKey()
		vals[i] = core.NewValidator(keys[i].PubKey(), p)
		byAddr[string(keys[i].PubKey().Address())] = keys[i]
	}
	valSet := core.NewValidatorSet(vals)
	accounts := testfactory.GenerateAccounts(m)
	testApp := testutil.NewTestApp()
	genesisState, kr := testutil.GenesisStateWithValidatorSet(testApp, valSet, accounts...)
	var slashingGenesis slashingtypes.GenesisState
	testApp.AppCodec().MustUnmarshalJSON(genesisState[slashingtypes.ModuleName], &slashingGenesis)
	for _, key := range keys {
		consAddr := sdk.ConsAddress(key.PubKey().Address())
		slashingGenesis.SigningInfos = append(slashingGenesis.SigningInfos, slashingtypes.SigningInfo{
			Address:              consAddr.String(),
			ValidatorSigningInfo: slashingtypes.NewValidatorSigningInfo(consAddr, 0, 0, time.Unix(0, 0).UTC(), false, 0),
		})
	}
	genesisState[slashingtypes.ModuleName] = json.RawMessage(testApp.AppCodec().MustMarshalJSON(&slashingGenesis))
	testutil.InitialiseTestAppWithGenesis(testApp, app.DefaultConsensusParams(), genesisState)

	votes := make([]abci.VoteInfo, len(valSet.Validators))
	for i, v := range valSet.Validators {
		votes[i] = abci.VoteInfo{Validator: abci.Validator{Address: v.Address, Power: v.VotingPower}, BlockIdFlag: cmtproto.BlockIDFlagCommit}
	}
	blockTime := testutil.GenesisTime
	finalize := func(txs [][]byte) (fb, commit time.Duration) {
		blockTime = blockTime.Add(time.Second)
		t0 := time.Now()
		resp, err := testApp.FinalizeBlock(&abci.RequestFinalizeBlock{
			Time: blockTime, Height: testApp.LastBlockHeight() + 1, Hash: testApp.LastCommitID().Hash,
			NextValidatorsHash: valSet.Hash(), DecidedLastCommit: abci.CommitInfo{Votes: votes}, Txs: txs,
		})
		fb = time.Since(t0)
		require.NoError(t, err)
		for i, r := range resp.TxResults {
			require.Equal(t, abci.CodeTypeOK, r.Code, "tx %d: %s", i, r.Log)
		}
		t1 := time.Now()
		_, err = testApp.Commit()
		require.NoError(t, err)
		return fb, time.Since(t1)
	}
	finalize(nil)
	signers := make([]*user.Signer, m)
	infos := queryAccountInfo(testApp, accounts, kr)
	for i := range accounts {
		addr := testfactory.GetAddress(kr, accounts[i])
		seedFibreEscrow(t, testApp, addr, 1_000_000_000_000)
		s, err := user.NewSigner(kr, enc.TxConfig, testutil.ChainID, user.NewAccount(accounts[i], infos[i].AccountNum, infos[i].Sequence))
		require.NoError(t, err)
		signers[i] = s
	}
	finalize(nil)
	var emptyFB []time.Duration
	for range 5 {
		fb, _ := finalize(nil)
		emptyFB = append(emptyFB, fb)
	}

	type res struct {
		Name                                                    string
		Vals, Sigs, Txs, TxBytes                                int
		CheckTxUs, PPMs, FBMs, CommitMs, EmptyFBMs, VerifyOneUs float64
	}
	r := res{Name: name, Vals: len(powers), Txs: m}
	var ck, pp, fb, cm []time.Duration
	for round := 0; round < rounds; round++ {
		h := testApp.LastBlockHeight()
		txs := make([][]byte, m)
		for i := range m {
			acc := signers[i].Account(accounts[i])
			msg := blobfactory.NewMsgPayForFibre(t, acc.PubKey().(*secp256k1.PubKey), testutil.ChainID)
			msg.PaymentPromise.Height = h
			msg.PaymentPromise.BlobSize = 262144
			msg.PaymentPromise.CreationTimestamp = blockTime
			_, _ = rand.Read(msg.PaymentPromise.Commitment)
			pp := fibre.PaymentPromise{}
			require.NoError(t, pp.FromProto(&msg.PaymentPromise))
			signBytes, err := pp.SignBytes()
			require.NoError(t, err)
			msg.PaymentPromise.Signature, _, err = signers[i].Keyring().Sign(accounts[i], signBytes, signing.SignMode_SIGN_MODE_DIRECT)
			require.NoError(t, err)
			set := validator.Set{ValidatorSet: valSet, Height: uint64(h)}
			sigSet := set.NewSignatureSet(fibre.DefaultProtocolParams.SafetyThreshold, signBytes)
			n := 0
			for _, v := range valSet.Validators {
				sig, err := byAddr[string(v.Address)].Sign(signBytes)
				require.NoError(t, err)
				n++
				if done, err := sigSet.Add(v, sig); err == nil && done {
					break
				}
			}
			r.Sigs = n
			msg.ValidatorSignatures, err = sigSet.Signatures()
			require.NoError(t, err)
			txs[i], _, err = signers[i].CreateTx([]sdk.Msg{msg}, user.SetGasLimit(400_000), user.SetFee(4_000))
			require.NoError(t, err)
			require.NoError(t, signers[i].IncrementSequence(accounts[i]))
			r.TxBytes += len(txs[i])
		}
		t0 := time.Now()
		for i, tx := range txs {
			resp, err := testApp.CheckTx(&abci.RequestCheckTx{Tx: tx, Type: abci.CheckTxType_New})
			require.NoError(t, err)
			require.Equal(t, abci.CodeTypeOK, resp.Code, "checktx %d: %s", i, resp.Log)
		}
		ck = append(ck, time.Since(t0)/time.Duration(m))
		req := processProposalRequestAt(t, testApp, txs, blockTime.Add(time.Second))
		for range 3 {
			t1 := time.Now()
			resp, err := testApp.ProcessProposal(req)
			pp = append(pp, time.Since(t1))
			require.NoError(t, err)
			require.Equal(t, abci.ResponseProcessProposal_ACCEPT, resp.Status)
		}
		f, c := finalize(txs)
		fb, cm = append(fb, f), append(cm, c)
	}
	// One ed25519 verify on these CPUs, for the per-signature slope.
	k := ed25519.GenPrivKey()
	msgBytes := make([]byte, 200)
	sig, _ := k.Sign(msgBytes)
	t0 := time.Now()
	for range 2000 {
		k.PubKey().VerifySignature(msgBytes, sig)
	}
	r.VerifyOneUs = float64(time.Since(t0).Microseconds()) / 2000
	med := func(xs []time.Duration) float64 {
		sort.Slice(xs, func(i, j int) bool { return xs[i] < xs[j] })
		return float64(xs[len(xs)/2].Microseconds()) / 1000
	}
	r.TxBytes /= m * rounds
	r.CheckTxUs, r.PPMs, r.FBMs, r.CommitMs, r.EmptyFBMs = med(ck)*1000, med(pp), med(fb), med(cm), med(emptyFB)
	out, _ := json.Marshal(r)
	fmt.Println("M5A_RESULT", string(out))
}
