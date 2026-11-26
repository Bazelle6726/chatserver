package ratelimit

import (
	"testing"
	"time"
)

// The package promises "N a second on average, in bursts of up to M", and
// that forgetting an idle key "loses nothing" (a bucket idle that long is
// full anyway). Together: in any stretch of time T, a key gets at most
// burst + perSecond*T events allowed. Check it for any rate, burst and
// pattern of requests, including gaps long enough for the sweep to forget
// the key.
//
//	go test ./internal/ratelimit -run XXX -fuzz FuzzNeverMoreThanTheBucket -fuzztime 50s
func FuzzNeverMoreThanTheBucket(f *testing.F) {
	f.Add(uint8(10), uint8(3), []byte{0, 0, 0, 0, 50, 0, 0, 255, 255, 0, 0, 0})
	f.Add(uint8(1), uint8(1), []byte{0, 1, 0, 200, 0})
	f.Fuzz(func(t *testing.T, rate, burst uint8, gaps []byte) {
		if rate == 0 || burst == 0 || len(gaps) > 400 {
			t.Skip()
		}
		perSecond := float64(rate) / 4 // 0.25 to 63.75 a second
		c := newClock()
		l := newTest(perSecond, int(burst), c)

		var allowed []time.Time
		for _, g := range gaps {
			// Mostly short steps; a high byte is a long pause (up to ~2 min),
			// past the idle time, so the key gets forgotten and recreated.
			step := time.Duration(g) * 10 * time.Millisecond
			if g > 240 {
				step = time.Duration(g-240) * 8 * time.Second
			}
			c.add(step)
			if l.Allow("k") {
				allowed = append(allowed, c.now())
			}
		}
		for i := range allowed {
			for j := range i + 1 {
				count := i - j + 1
				limit := float64(burst) + perSecond*allowed[i].Sub(allowed[j]).Seconds()
				if float64(count) > limit+1e-6 {
					t.Fatalf("%d events allowed between %v and %v; the bucket allows at most %.3f (rate %.2f/s, burst %d)",
						count, allowed[j].Format("15:04:05.000"), allowed[i].Format("15:04:05.000"), limit, perSecond, burst)
				}
			}
		}
	})
}
