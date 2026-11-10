package keeper

import (
	"crypto/rand"
	"math/big"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/celestiaorg/celestia-app/v11/x/fibre/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// Bench only: CELESTIA_BENCH_FILL=<total>:<per block> adds synthetic processed payments in
// FinalizeBlock, spread over the retention window, to emulate a steady-state store.
var benchFillTotal, benchFillPerBlock, benchFilled = func() (int64, int64, *atomic.Int64) {
	t, p, _ := strings.Cut(os.Getenv("CELESTIA_BENCH_FILL"), ":")
	total, _ := strconv.ParseInt(t, 10, 64)
	per, _ := strconv.ParseInt(p, 10, 64)
	return total, per, new(atomic.Int64)
}()

func (k Keeper) benchFill(ctx sdk.Context) {
	if benchFillTotal <= 0 || ctx.ExecMode() != sdk.ExecModeFinalize || benchFilled.Load() >= benchFillTotal {
		return
	}
	window := k.GetParams(ctx).PaymentPromiseRetentionWindow() - 5*time.Minute
	n := min(benchFillPerBlock, benchFillTotal-benchFilled.Load())
	for range n {
		hash := make([]byte, 32)
		_, _ = rand.Read(hash)
		age, _ := rand.Int(rand.Reader, big.NewInt(int64(window)))
		k.SetProcessedPayment(ctx, types.ProcessedPayment{PaymentPromiseHash: hash, ProcessedAt: ctx.BlockTime().Add(-time.Duration(age.Int64()))})
	}
	k.Logger(ctx).Info("bench fill", "filled", benchFilled.Add(n), "total", benchFillTotal)
}
