package appconsts

import "os"

// BenchLegacyTimeouts makes v11+ return the v10 timeouts (no dynamic floor).
// Bench-only: set CELESTIA_BENCH_LEGACY_TIMEOUTS=1; read once at start.
var BenchLegacyTimeouts = os.Getenv("CELESTIA_BENCH_LEGACY_TIMEOUTS") == "1"
