package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

const utiaPerTIA = 1_000_000

// writeValidatorPower prints the genesis voting power per validator and writes
// it to <payload>/validator_power.csv (name,region,stake_utia,power).
func writeValidatorPower(ppath string, ips []Instance, stakes []int64) error {
	powers := make([]int64, len(stakes))
	var total int64
	for i, s := range stakes {
		powers[i] = sdk.TokensToConsensusPower(math.NewInt(s), sdk.DefaultPowerReduction)
		total += powers[i]
	}
	var csv strings.Builder
	csv.WriteString("name,region,stake_utia,power\n")
	for i, info := range ips {
		fmt.Fprintf(&csv, "%s,%s,%d,%d\n", info.Name, info.Region, stakes[i], powers[i])
	}
	order := make([]int, len(ips))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return powers[order[a]] > powers[order[b]] })
	var cum int64
	fmt.Printf("%-4s %-24s %-16s %14s %8s\n", "rank", "validator", "region", "power", "cum%")
	for rank, i := range order {
		cum += powers[i]
		fmt.Printf("%-4d %-24s %-16s %14d %7.2f%%\n", rank+1, ips[i].Name, ips[i].Region, powers[i], 100*float64(cum)/float64(max(total, 1)))
	}
	return os.WriteFile(filepath.Join(ppath, "validator_power.csv"), []byte(csv.String()), 0o644)
}
