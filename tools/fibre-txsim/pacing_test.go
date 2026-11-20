package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestWaitNextFixedSchedule checks the offered rate ignores submit latency below the interval.
func TestWaitNextFixedSchedule(t *testing.T) {
	liveInterval.Store(int64(40 * time.Millisecond))
	start := time.Now()
	next := start
	for range 10 {
		time.Sleep(25 * time.Millisecond) // submit latency
		require.True(t, waitNext(context.Background(), &next))
	}
	require.InDelta(t, 400*time.Millisecond, time.Since(start), float64(60*time.Millisecond))
}

func TestReadIntervalFile(t *testing.T) {
	intervalFile = filepath.Join(t.TempDir(), "interval")
	t.Cleanup(func() { intervalFile = "" })
	liveInterval.Store(int64(time.Second))

	require.NoError(t, os.WriteFile(intervalFile, []byte("250ms\n"), 0o600))
	readIntervalFile()
	require.Equal(t, 250*time.Millisecond, time.Duration(liveInterval.Load()))

	require.NoError(t, os.WriteFile(intervalFile, []byte("bogus"), 0o600))
	readIntervalFile()
	require.Equal(t, 250*time.Millisecond, time.Duration(liveInterval.Load()), "an invalid file keeps the last interval")
}
