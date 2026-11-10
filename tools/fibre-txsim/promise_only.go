package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/celestiaorg/celestia-app/v11/fibre"
	fibretypes "github.com/celestiaorg/celestia-app/v11/x/fibre/types"
	"github.com/celestiaorg/go-square/v4/share"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// promiseOnly sends payment promises with a random commitment and no data
// (bench only; validators need CELESTIA_BENCH_FIBRE_PROMISE_ONLY=1).
var promiseOnly bool

func init() {
	flag.BoolVar(&promiseOnly, "promise-only", false, "bench: send only the payment promise (random commitment, no encoding/shards) for --blob-size bytes; validators must run fibre with CELESTIA_BENCH_FIBRE_PROMISE_ONLY=1")
}

func submitPromiseOnly(ctx context.Context, w worker, ns share.Namespace, blobSize int, uploadOnly bool, st *stats, confirmCh chan<- confirmRequest) {
	st.totalSent.Add(1)
	t := time.Now()
	signedPromise, err := w.fibreClient.UploadPromiseOnly(ctx, ns, blobSize, fibre.WithKeyName(w.keyName))
	if err != nil {
		if ctx.Err() == nil {
			st.failures.Add(1)
			fmt.Printf("[%s] upload error: %v\n", w.keyName, err)
		}
		return
	}
	if uploadOnly {
		st.successes.Add(1)
		st.totalLatNs.Add(time.Since(t).Nanoseconds())
		fmt.Printf("[%s] upload-only: latency=%s\n", w.keyName, time.Since(t))
		return
	}
	promiseProto, err := signedPromise.ToProto()
	if err != nil {
		st.failures.Add(1)
		fmt.Printf("[%s] promise proto error: %v\n", w.keyName, err)
		return
	}
	msg := &fibretypes.MsgPayForFibre{
		Signer:              w.txClient.DefaultAddress().String(),
		PaymentPromise:      *promiseProto,
		ValidatorSignatures: signedPromise.ValidatorSignatures,
	}
	broadcastResp, err := w.txClient.BroadcastTx(ctx, []sdk.Msg{msg})
	if err != nil {
		if ctx.Err() == nil {
			st.failures.Add(1)
			fmt.Printf("[%s] broadcast error: %v\n", w.keyName, err)
		}
		return
	}
	uploadLat := time.Since(t)
	st.successes.Add(1)
	st.totalLatNs.Add(uploadLat.Nanoseconds())
	st.uploadLatNs.Add(uploadLat.Nanoseconds())
	st.uploadCount.Add(1)
	fmt.Printf("[%s] broadcast: tx=%s upload_latency=%s\n", w.keyName, broadcastResp.TxHash, uploadLat)
	if confirmCh != nil {
		select {
		case confirmCh <- confirmRequest{grpcConn: w.grpcConn, txHash: broadcastResp.TxHash, keyName: w.keyName, startT: t}:
		default:
		}
	}
}
