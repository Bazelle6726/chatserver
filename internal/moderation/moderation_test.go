package moderation

import (
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
)

func TestIsBanned(t *testing.T) {
	l := New("scam", "Spam")
	tests := []struct {
		text string
		want bool
	}{
		{"cheap scam here", true},
		{"SCAM!", true},
		{"this is spam.", true},
		{"(scam)", true},
		{"scampi for dinner", false},
		{"spammy", false},
		{"hello world", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := l.IsBanned(tt.text); got != tt.want {
			t.Errorf("IsBanned(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

func TestAddRemove(t *testing.T) {
	l := New()
	l.Add("  Scam ")
	if !l.IsBanned("a scam") {
		t.Error("Add didn't ban the word")
	}
	l.Remove("SCAM")
	if l.IsBanned("a scam") {
		t.Error("Remove didn't unban the word")
	}
	l.Add("")
	if len(l.Words()) != 0 {
		t.Errorf("an empty word was added: %v", l.Words())
	}
}

func TestLoadAndSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "words.txt")
	os.WriteFile(path, []byte("# a comment\n\nscam\n  Spam  \n# another\nfraud\n"), 0o644)

	l, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := l.Words(), []string{"fraud", "scam", "spam"}; !slices.Equal(got, want) {
		t.Fatalf("Load = %v, want %v", got, want)
	}

	l.Add("malware")
	out := filepath.Join(dir, "saved.txt")
	if err := l.Save(out); err != nil {
		t.Fatal(err)
	}
	again, err := Load(out)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(again.Words(), l.Words()) {
		t.Errorf("saved and reloaded = %v, want %v", again.Words(), l.Words())
	}

	if _, err := Load(filepath.Join(dir, "missing.txt")); err == nil {
		t.Error("Load of a missing file should fail")
	}
}

// Workers read while someone adds and removes: -race checks the locking.
func TestConcurrentUse(t *testing.T) {
	l := New("scam")
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 1000 {
				if i%2 == 0 {
					l.IsBanned("a scam here")
				} else {
					l.Add("spam")
					l.Remove("spam")
				}
			}
		}()
	}
	wg.Wait()
}
