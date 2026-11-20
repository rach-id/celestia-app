package appconsts

import (
	"os"
	"strconv"
	"strings"
)

const (
	// BenchMaxPayForFibreMessages is the bench ProcessProposal PayForFibre limit (mainnet 200).
	BenchMaxPayForFibreMessages = 1000
	// BenchMaxPayForFibreCandidates bounds a pre-verification pass at the bench limit.
	BenchMaxPayForFibreCandidates = 2 * BenchMaxPayForFibreMessages
	// BenchMaxPFFFile holds the proposer's live PayForFibre cap; CELESTIA_BENCH_MAX_PFF_FILE overrides it.
	BenchMaxPFFFile = "/root/bench/max_pff"
)

// ProposerMaxPayForFibreMessages reads the proposer's cap on every call: the file's integer
// clamped to [1, BenchMaxPayForFibreMessages], or 200 if the file is missing or invalid.
func ProposerMaxPayForFibreMessages() int {
	path := BenchMaxPFFFile
	if p := os.Getenv("CELESTIA_BENCH_MAX_PFF_FILE"); p != "" {
		path = p
	}
	fallback := min(MaxPayForFibreMessages, BenchMaxPayForFibreMessages)
	b, err := os.ReadFile(path)
	if err != nil {
		return fallback
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return fallback
	}
	return min(max(n, 1), BenchMaxPayForFibreMessages)
}
