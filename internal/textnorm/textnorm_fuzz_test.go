package textnorm

import (
	"testing"
	"unicode"
	"unicode/utf8"
)

// The package comment promises that strings which LOOK the same get the same
// skeleton. These fuzz tests turn that promise into properties the computer
// checks on millions of generated strings, instead of the handful in the
// table test.
//
//	go test ./internal/textnorm -run XXX -fuzz FuzzSkeletonIdempotent -fuzztime 30s
//	go test ./internal/textnorm -run XXX -fuzz FuzzInvisibleCharsDontMatter -fuzztime 30s
//
// Plain `go test ./...` runs every seed below (and any saved failures in
// testdata/fuzz) as ordinary tests, so a bug found once stays fixed.

var seeds = []string{
	"scam", "Scam", "ｓｃａｍ", "sсam", "sc​am", "sc­am",
	"pаypal", "café", "café", "ﬁsh", "𝐬cam", "İstanbul", "Ωmega",
}

// Folding an already-folded string must change nothing. If it does, two
// strings can look identical but only match after a second fold, so a name
// or banned word can slip past a single Skeleton call.
func FuzzSkeletonIdempotent(f *testing.F) {
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if !utf8.ValidString(s) {
			t.Skip()
		}
		once := Skeleton(s)
		twice := Skeleton(once)
		if once != twice {
			t.Errorf("Skeleton is not idempotent\n input   %+q\n once    %+q\n twice   %+q", s, once, twice)
		}
		for _, r := range once {
			if unicode.Is(unicode.Cf, r) {
				t.Errorf("Skeleton(%+q) = %+q still contains invisible %U", s, once, r)
			}
		}
	})
}

// An invisible character must not change how a string folds: the package
// comment says "sc​am" (zero-width space) folds to "scam". Check it for
// any string and any position, not just that one example.
func FuzzInvisibleCharsDontMatter(f *testing.F) {
	for _, s := range seeds {
		f.Add(s, uint8(1))
	}
	f.Fuzz(func(t *testing.T, s string, at uint8) {
		if !utf8.ValidString(s) || s == "" {
			t.Skip()
		}
		runes := []rune(s)
		i := int(at) % (len(runes) + 1)
		disguised := string(runes[:i]) + "​" + string(runes[i:])
		if got, want := Skeleton(disguised), Skeleton(s); got != want {
			t.Errorf("a zero-width space changed the skeleton\n input      %+q\n disguised  %+q\n Skeleton   %+q, want %+q", s, disguised, got, want)
		}
	})
}
