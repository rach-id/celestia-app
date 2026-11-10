package app_test

import (
	"testing"
	"time"

	"cosmossdk.io/log"
	"github.com/celestiaorg/celestia-app/v11/app"
	"github.com/celestiaorg/celestia-app/v11/pkg/appconsts"
	"github.com/celestiaorg/celestia-app/v11/test/util"
	tmdb "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/stretchr/testify/require"
)

func TestBenchLegacyTimeoutInfo(t *testing.T) {
	orig := appconsts.BenchLegacyTimeouts
	t.Cleanup(func() { appconsts.BenchLegacyTimeouts = orig })
	appconsts.BenchLegacyTimeouts = true

	testApp := app.New(log.NewNopLogger(), tmdb.NewMemDB(), nil, 0, 0, util.EmptyAppOptions{}, baseapp.SetChainID(util.ChainID))
	testApp.SetDynamicTimeoutCommit(500 * time.Millisecond) // must not re-enable the floor
	require.Equal(t, v10TimeoutInfo, testApp.TimeoutInfo(11))
}
