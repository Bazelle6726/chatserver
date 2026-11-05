package main

import (
	"testing"
	"time"
)

func TestPercentile(t *testing.T) {
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	ten := []time.Duration{ms(10), ms(20), ms(30), ms(40), ms(50), ms(60), ms(70), ms(80), ms(90), ms(100)}

	tests := []struct {
		name   string
		sorted []time.Duration
		p      float64
		want   time.Duration
	}{
		{"p50 of 10", ten, 50, ms(50)},
		{"p90 of 10", ten, 90, ms(90)},
		{"p95 of 10 rounds up", ten, 95, ms(100)},
		{"p99 of 10", ten, 99, ms(100)},
		{"p100 is the max", ten, 100, ms(100)},
		{"p0 is the min", ten, 0, ms(10)},
		{"p1 of 10", ten, 1, ms(10)},
		{"one value", []time.Duration{ms(7)}, 99, ms(7)},
		{"empty", nil, 50, 0},
	}
	for _, tt := range tests {
		if got := percentile(tt.sorted, tt.p); got != tt.want {
			t.Errorf("%s: percentile(p=%v) = %v, want %v", tt.name, tt.p, got, tt.want)
		}
	}
}
