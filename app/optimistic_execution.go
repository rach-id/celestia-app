package app

import (
	"github.com/celestiaorg/celestia-app/v11/pkg/appconsts"
	coretypes "github.com/cometbft/cometbft/types"
)

// stripBlobTxs returns the txs as celestia-core hands them to FinalizeBlock: BlobTx
// envelopes replaced by their inner tx (state/execution.go applyBlock).
func stripBlobTxs(txs [][]byte) [][]byte {
	stripped := make([][]byte, len(txs))
	for i, tx := range txs {
		// BENCH ONLY: core main unwraps by app version; assumes a v11 chain.
		stripped[i], _ = coretypes.UnwrapBlobTx(tx, appconsts.Version)
	}
	return stripped
}
