package moderation

import (
	"path/filepath"
	"slices"
	"testing"
)

// Saving the list writes it to its file "so changes survive a restart"
// (service.saveWords). So whatever an admin has banned must come back
// exactly from Load after Save: no word lost, none changed, none added.
//
//	go test ./internal/moderation -run XXX -fuzz FuzzSaveThenLoadKeepsTheList -fuzztime 50s
func FuzzSaveThenLoadKeepsTheList(f *testing.F) {
	f.Add("scam", "Fr4ud")
	f.Add("ѕсаm", "café")
	f.Add("#hash", "spam")
	f.Fuzz(func(t *testing.T, a, b string) {
		l := New()
		l.Add(a)
		l.Add(b)
		path := filepath.Join(t.TempDir(), "banned_words.txt")
		if err := l.Save(path); err != nil {
			t.Fatal(err)
		}
		again, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := again.Words(), l.Words(); !slices.Equal(got, want) {
			t.Errorf("after Add(%+q), Add(%+q): saved %+q, loaded back %+q", a, b, want, got)
		}
	})
}
