package textnorm

import "testing"

func TestSkeleton(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Scam", "scam"},
		{"ｓｃａｍ", "scam"},   // fullwidth
		{"𝐬𝐜𝐚𝐦", "scam"},   // mathematical bold
		{"sсam", "scam"},   // Cyrillic с
		{"ѕсаm", "scam"},   // Cyrillic ѕ, с, а
		{"sc​am", "scam"},  // zero-width space
		{"sc­am", "scam"},  // soft hyphen
		{"АLICE", "alice"}, // Cyrillic А, uppercase
		{"ВОВ", "bob"},     // all Cyrillic, looks like BOB
		{"Zoë", "zoë"},     // a real accent stays
		{"hello world", "hello world"},
	}
	for _, tt := range tests {
		if got := Skeleton(tt.in); got != tt.want {
			t.Errorf("Skeleton(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestMixedScripts(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"alice", false},
		{"Zoë", false},
		{"алиса", false},  // all Cyrillic
		{"αλφα", false},   // all Greek
		{"аlice", true},   // Cyrillic а + Latin
		{"bob_99", false}, // digits and symbols belong to no script
		{"山田たろう", false},  // kanji + hiragana is normal Japanese
	}
	for _, tt := range tests {
		if got := MixedScripts(tt.in); got != tt.want {
			t.Errorf("MixedScripts(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
