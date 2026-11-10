//go:build !benchmarks

package appconsts

import (
	"os"
	"strconv"
)

var (
	// MaxPayForFibreMessages is the maximum number of PayForFibre messages that a block can contain.
	// Bench only: 600 (mainnet 200), overridable with CELESTIA_BENCH_MAX_PFF for the 5a-M cells.
	MaxPayForFibreMessages = benchMaxPFF()
	// MaxPayForFibreCandidates is the number of PayForFibre transactions a proposer pre-verifies
	// for one block. It bounds that warm-up, not inclusion: the fill keeps going past it.
	MaxPayForFibreCandidates = 2 * MaxPayForFibreMessages
)

func benchMaxPFF() int {
	if n, err := strconv.Atoi(os.Getenv("CELESTIA_BENCH_MAX_PFF")); err == nil && n > 0 {
		return n
	}
	return 600
}
