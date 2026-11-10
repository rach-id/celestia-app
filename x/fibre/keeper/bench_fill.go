package keeper

import (
	"crypto/rand"
	"encoding/binary"
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
	window := k.GetParams(ctx).PaymentPromiseRetentionWindow() - 15*time.Minute
	n := min(benchFillPerBlock, benchFillTotal-benchFilled.Load())
	// Ascending keys spread uniformly over the hash space and the window: the tree ends up
	// as random inserts would leave it, but each insert walks the cached rightmost path.
	step := ^uint64(0) / uint64(benchFillTotal)
	start := ctx.BlockTime().Add(-window)
	for i := benchFilled.Load(); i < benchFilled.Load()+n; i++ {
		hash := make([]byte, 32)
		_, _ = rand.Read(hash[8:])
		binary.BigEndian.PutUint64(hash, uint64(i)*step)
		at := start.Add(time.Duration(float64(window) * float64(i) / float64(benchFillTotal)))
		k.SetProcessedPayment(ctx, types.ProcessedPayment{PaymentPromiseHash: hash, ProcessedAt: at})
	}
	k.Logger(ctx).Info("bench fill", "filled", benchFilled.Add(n), "total", benchFillTotal)
}
