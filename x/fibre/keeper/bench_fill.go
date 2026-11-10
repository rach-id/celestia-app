package keeper

import (
	"crypto/sha256"
	"encoding/binary"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/celestiaorg/celestia-app/v11/x/fibre/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// Bench only: CELESTIA_BENCH_FILL=<total>:<per block>:<start height> adds synthetic processed
// payments in FinalizeBlock, a deterministic function of the height, to emulate a steady-state store.
var benchFillTotal, benchFillPerBlock, benchFillStart = func() (int64, int64, int64) {
	f := strings.Split(os.Getenv("CELESTIA_BENCH_FILL"), ":")
	if len(f) != 3 {
		return 0, 0, 0
	}
	total, _ := strconv.ParseInt(f[0], 10, 64)
	per, _ := strconv.ParseInt(f[1], 10, 64)
	start, _ := strconv.ParseInt(f[2], 10, 64)
	return total, per, start
}()

func (k Keeper) benchFill(ctx sdk.Context) {
	first := (ctx.BlockHeight() - benchFillStart) * benchFillPerBlock
	if benchFillTotal <= 0 || ctx.ExecMode() != sdk.ExecModeFinalize || first < 0 || first >= benchFillTotal {
		return
	}
	window := k.GetParams(ctx).PaymentPromiseRetentionWindow() - 15*time.Minute
	// Ascending keys spread uniformly over the hash space and the window: the tree ends up
	// as random inserts would leave it, but each insert walks the cached rightmost path.
	step := ^uint64(0) / uint64(benchFillTotal)
	start := ctx.BlockTime().Add(-window)
	last := min(first+benchFillPerBlock, benchFillTotal)
	for i := first; i < last; i++ {
		hash := sha256.Sum256(binary.BigEndian.AppendUint64(nil, uint64(i)))
		binary.BigEndian.PutUint64(hash[:8], uint64(i)*step)
		at := start.Add(time.Duration(float64(window) * float64(i) / float64(benchFillTotal)))
		k.SetProcessedPayment(ctx, types.ProcessedPayment{PaymentPromiseHash: hash[:], ProcessedAt: at})
	}
	k.Logger(ctx).Info("bench fill", "filled", last, "total", benchFillTotal)
}
