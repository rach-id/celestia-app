package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

// Bench: workers start on a fixed schedule (offered rate = concurrency/interval, independent of
// submit latency); --interval-file changes the interval live and stats print every 5 s.
var (
	intervalFile string
	liveInterval atomic.Int64
)

func init() {
	flag.StringVar(&intervalFile, "interval-file", "", "bench: file holding the per-worker interval (e.g. 500ms), re-read every second; overrides --interval")
}

// startPacing seeds the live interval and, with --interval-file, re-reads it every second.
func startPacing(ctx context.Context, interval time.Duration) {
	liveInterval.Store(int64(interval))
	if intervalFile == "" {
		return
	}
	readIntervalFile()
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				readIntervalFile()
			}
		}
	}()
}

func readIntervalFile() {
	b, err := os.ReadFile(intervalFile)
	if err != nil {
		return
	}
	d, err := time.ParseDuration(strings.TrimSpace(string(b)))
	if err != nil || d < 0 {
		return
	}
	if old := time.Duration(liveInterval.Swap(int64(d))); old != d {
		fmt.Printf("interval: %s -> %s\n", old, d)
	}
}

// waitNext sleeps until the worker's next start, a fixed interval after the previous one. A
// worker that falls behind starts at once without bursting to catch up. False when ctx is done.
func waitNext(ctx context.Context, next *time.Time) bool {
	iv := time.Duration(liveInterval.Load())
	if iv <= 0 {
		*next = time.Now()
		return ctx.Err() == nil
	}
	*next = next.Add(iv)
	d := time.Until(*next)
	if d <= 0 {
		*next = time.Now()
		return ctx.Err() == nil
	}
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

// reportRates prints sent/ok/fail per second and the mean submit latency every 5 s.
func reportRates(ctx context.Context, st *stats) {
	const every = 5 * time.Second
	t := time.NewTicker(every)
	defer t.Stop()
	var sent, ok, fail, lat int64
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		s, o, f, l := st.totalSent.Load(), st.successes.Load(), st.failures.Load(), st.totalLatNs.Load()
		var avg time.Duration
		if o > ok {
			avg = time.Duration((l - lat) / (o - ok))
		}
		secs := every.Seconds()
		fmt.Printf("rate: sent=%.1f/s ok=%.1f/s fail=%.1f/s avg_latency=%s interval=%s\n",
			float64(s-sent)/secs, float64(o-ok)/secs, float64(f-fail)/secs, avg.Truncate(time.Millisecond), time.Duration(liveInterval.Load()))
		sent, ok, fail, lat = s, o, f, l
	}
}
