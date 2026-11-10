package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
)

func TestMainnetStakeDistribution(t *testing.T) {
	const n = 100
	powers := make([]int64, n)
	var total int64
	for i := range n {
		powers[i] = sdk.TokensToConsensusPower(math.NewInt(getMainnetStake(i)), sdk.DefaultPowerReduction)
		require.Positive(t, powers[i], "validator %d has zero power", i)
		total += powers[i]
	}
	topShare := func(k int) float64 {
		var s int64
		for _, p := range powers[:k] { // the list is sorted by stake
			s += p
		}
		return float64(s) / float64(total)
	}
	require.Less(t, topShare(5), 1.0/3)
	require.Greater(t, topShare(6), 1.0/3)
	require.Less(t, topShare(25), 2.0/3)
	require.Greater(t, topShare(26), 2.0/3)
}

func TestWriteValidatorPower(t *testing.T) {
	dir := t.TempDir()
	ips := []Instance{{Name: "validator-0", Region: "us-east1"}, {Name: "validator-1", Region: "asia-east1"}}
	require.NoError(t, writeValidatorPower(dir, ips, []int64{getMainnetStake(0), getMainnetStake(99)}))
	b, err := os.ReadFile(filepath.Join(dir, "validator_power.csv"))
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	require.Equal(t, []string{
		"name,region,stake_utia,power",
		fmt.Sprintf("validator-0,us-east1,%d,44706511", getMainnetStake(0)),
		fmt.Sprintf("validator-1,asia-east1,%d,445353", getMainnetStake(99)),
	}, lines)
}
