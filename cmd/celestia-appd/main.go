// Overrides go.work too, which ignores go.mod godebug lines.

//go:debug multipathtcp=0
package main

import (
	"os"
	"runtime"
	"strconv"

	"github.com/celestiaorg/celestia-app/v11/app"
	"github.com/celestiaorg/celestia-app/v11/cmd/celestia-appd/cmd"
	svrcmd "github.com/cosmos/cosmos-sdk/server/cmd"
)

func main() {
	// bench only (core3764 A/B): block profile for /debug/pprof/block, rate in ns.
	if r, err := strconv.Atoi(os.Getenv("BENCH_BLOCK_PROFILE_RATE")); err == nil && r > 0 {
		runtime.SetBlockProfileRate(r)
	}
	rootCmd := cmd.NewRootCmd()
	if err := svrcmd.Execute(rootCmd, app.EnvPrefix, app.NodeHome); err != nil {
		os.Exit(1)
	}
}
