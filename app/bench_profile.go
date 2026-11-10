package app

import (
	"os"
	"runtime"
	"strconv"
)

// Bench only: CELESTIA_BENCH_MUTEX_FRACTION=<n> enables mutex and block profiling (1 in n events).
func init() {
	if n, err := strconv.Atoi(os.Getenv("CELESTIA_BENCH_MUTEX_FRACTION")); err == nil && n > 0 {
		runtime.SetMutexProfileFraction(n)
		runtime.SetBlockProfileRate(n * 1000)
	}
}
