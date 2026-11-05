package main

import (
	"time"
)

// percentile returns the p-th percentile (0-100) of sorted.
//
// A percentile answers "how long did the slowest few take?". p50 (the
// median) is a typical request; p99 is the one-in-a-hundred slow one, which
// averages hide.
//
// TODO(70): use the "nearest rank" method:
//   - an empty slice: return 0
//   - rank := ceil(p / 100 * len(sorted))   (math.Ceil works on float64,
//     so convert: float64(len(sorted)), then int(...) the result)
//   - keep rank between 1 and len(sorted) (p0 would give 0): min and max
//   - return sorted[rank-1]   (ranks count from 1, indexes from 0)
//
// Example, 10 values: p50 -> rank 5 -> sorted[4]; p95 -> ceil(9.5) = 10 -> sorted[9].
//
// Check: go test ./cmd/loadtest -run Percentile -v
func percentile(sorted []time.Duration, p float64) time.Duration {
	return 0
}
