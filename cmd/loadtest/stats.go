package main

import (
	"math"
	"time"
)

// percentile returns the p-th percentile (0-100) of sorted.
//
// A percentile answers "how long did the slowest few take?". p50 (the
// median) is a typical request; p99 is the one-in-a-hundred slow one, which
// averages hide.
func percentile(sorted []time.Duration, p float64) time.Duration {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	rank := int(math.Ceil(p / 100 * float64(n)))
	if rank < 1 {
		rank = 1
	} else if rank > n {
		rank = n
	}
	return sorted[rank-1]
}
