package appconsts_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/celestiaorg/celestia-app/v11/pkg/appconsts"
	"github.com/stretchr/testify/require"
)

func TestProposerMaxPayForFibreMessages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "max_pff")
	t.Setenv("CELESTIA_BENCH_MAX_PFF_FILE", path)
	require.Equal(t, appconsts.MaxPayForFibreMessages, appconsts.ProposerMaxPayForFibreMessages(), "missing file")

	for content, want := range map[string]int{
		"300\n": 300,
		" 450 ": 450,
		"1000":  1000,
		"5000":  appconsts.BenchMaxPayForFibreMessages,
		"0":     1,
		"-7":    1,
		"abc":   appconsts.MaxPayForFibreMessages,
		"":      appconsts.MaxPayForFibreMessages,
	} {
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
		require.Equal(t, want, appconsts.ProposerMaxPayForFibreMessages(), "content %q", content)
	}
}
