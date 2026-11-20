package main

import (
	"os"
	"os/exec"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBenchProfileFraction re-runs the test binary with the env set, since init reads it at start.
func TestBenchProfileFraction(t *testing.T) {
	if os.Getenv("CELESTIA_BENCH_PROFILE_FRACTION") != "" {
		require.Equal(t, 7, runtime.SetMutexProfileFraction(-1))
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestBenchProfileFraction$")
	cmd.Env = append(os.Environ(), "CELESTIA_BENCH_PROFILE_FRACTION=7")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	require.Zero(t, runtime.SetMutexProfileFraction(-1), "unset by default")
}
