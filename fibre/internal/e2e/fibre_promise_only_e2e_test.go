package e2e_test

import (
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/celestiaorg/celestia-app/v11/app"
	"github.com/celestiaorg/celestia-app/v11/app/encoding"
	"github.com/celestiaorg/celestia-app/v11/fibre"
	"github.com/celestiaorg/celestia-app/v11/pkg/appconsts"
	"github.com/celestiaorg/celestia-app/v11/pkg/user"
	"github.com/celestiaorg/celestia-app/v11/test/util/testnode"
	fibretypes "github.com/celestiaorg/celestia-app/v11/x/fibre/types"
	"github.com/celestiaorg/go-square/v4/share"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
)

// TestFibrePromiseOnlyPayForFibre checks that a bench promise-only upload (random
// commitment, no shard) is signed by the server and its PayForFibre executes on chain.
func TestFibrePromiseOnlyPayForFibre(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping fibre promise-only e2e test in short mode")
	}
	ecfg := encoding.MakeConfig(app.ModuleEncodingRegisters...)
	cfg := testnode.DefaultConfig().
		WithFundedAccounts(fibre.DefaultKeyName).
		WithDelayedPrecommitTimeout(500 * time.Millisecond)
	cctx, _, grpcAddr := testnode.NewNetwork(t, cfg)
	ctx := cctx.GoContext()
	_, err := cctx.WaitForHeight(1)
	require.NoError(t, err)

	stack := startFibreStack(t, cctx, ecfg, grpcAddr)
	t.Cleanup(func() {
		_ = stack.server.Stop(ctx)
		_ = stack.client.Stop(ctx)
	})
	fundEscrow(t, cctx, stack.tx, sdk.NewCoin(appconsts.BondDenom, sdkmath.NewInt(50_000_000)))
	require.NoError(t, cctx.WaitForNextBlock())

	const dataSize = 256 << 10
	ns := share.MustNewV0Namespace([]byte{0x50, 0x4f})
	keyOpt := fibre.WithKeyName(stack.tx.DefaultAccountName())

	_, err = stack.client.UploadPromiseOnly(ctx, ns, dataSize, keyOpt)
	require.ErrorContains(t, err, "not enough", "without the bench flag a shardless upload must not be signed")

	fibre.BenchPromiseOnly = true
	t.Cleanup(func() { fibre.BenchPromiseOnly = false })
	signed, err := stack.client.UploadPromiseOnly(ctx, ns, dataSize, keyOpt)
	require.NoError(t, err)

	fibreQ := fibretypes.NewQueryClient(cctx.GRPCClient)
	signer := stack.tx.DefaultAddress().String()
	escrowBalance := func() sdk.Coin {
		resp, err := fibreQ.EscrowAccount(ctx, &fibretypes.QueryEscrowAccountRequest{Signer: signer})
		require.NoError(t, err)
		require.True(t, resp.Found)
		return resp.EscrowAccount.Balance
	}
	before := escrowBalance()

	pb, err := signed.ToProto()
	require.NoError(t, err)
	resp, err := stack.tx.SubmitTx(ctx, []sdk.Msg{&fibretypes.MsgPayForFibre{
		Signer:              signer,
		PaymentPromise:      *pb,
		ValidatorSignatures: signed.ValidatorSignatures,
	}}, user.SetGasLimit(1_000_000), user.SetFee(100_000))
	require.NoError(t, err)
	require.Equal(t, uint32(0), resp.Code)
	require.Positive(t, resp.Height)

	hash, err := signed.Hash()
	require.NoError(t, err)
	waitForPaymentProcessed(t, cctx, hash)
	wantDebit := fibretypes.PaymentAmount(uint32(fibre.DefaultBlobConfigV0().UploadSize(dataSize)))
	require.Equal(t, wantDebit, before.Sub(escrowBalance()), "escrow should be debited for the promised upload size")
	t.Logf("promise-only PFF included at height %d, upload_size %d, debit %s", resp.Height, signed.UploadSize, wantDebit)
}
