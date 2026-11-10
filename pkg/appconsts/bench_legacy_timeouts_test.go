package appconsts

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBenchLegacyTimeouts(t *testing.T) {
	orig := BenchLegacyTimeouts
	t.Cleanup(func() { BenchLegacyTimeouts = orig })

	BenchLegacyTimeouts = false
	require.NotZero(t, GetConsensusTimeouts(11).DynamicTimeoutCommit)

	BenchLegacyTimeouts = true
	got := GetConsensusTimeouts(11)
	require.Equal(t, v10ConsensusTimeouts, got)
	require.Zero(t, got.DynamicTimeoutCommit)
	require.Equal(t, v10ConsensusTimeouts, GetConsensusTimeouts(10))
}
