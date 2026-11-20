package main

import (
	"os"
	"runtime"
	"strconv"
)

// Bench: CELESTIA_BENCH_PROFILE_FRACTION=N sets the mutex profile fraction and block profile rate to N.
func init() {
	if n, err := strconv.Atoi(os.Getenv("CELESTIA_BENCH_PROFILE_FRACTION")); err == nil && n > 0 {
		runtime.SetMutexProfileFraction(n)
		runtime.SetBlockProfileRate(n)
	}
}
