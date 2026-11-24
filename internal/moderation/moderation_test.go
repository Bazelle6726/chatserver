package moderation

import (
	"fmt"
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

		// Disguises that used to slip through.
		{"cheap sc4m here", true},    // leetspeak digit
		{"$cam", true},               // leetspeak symbol
		{"sc@m", true},               // leetspeak symbol
		{"email bob@scam.com", true}, // a symbol that really is a separator
		{"s c a m", true},            // spaced out
		{"s.c.a.m!", true},           // dotted
		{"total a s c a m", true},    // spaced, after another single letter
		{"sc​am", true},              // zero-width space inside
		{"ѕсаm", true},               // Cyrillic look-alikes
		{"ＳＣＡＭ", true},               // fullwidth
		{"s̲c̲a̲m̲", true},           // combining underline

		// ...without new false alarms.
		{"I have 5 apples", false},
		{"a b c", false},
		{"$100 for scampi", false},
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

// Adding a disguised word bans the plain one, and is listed folded.
func TestAddFolds(t *testing.T) {
	l := New()
	if got := l.Add("Fr4ud"); got != "fraud" {
		t.Errorf("Add(%q) stored %q, want %q", "Fr4ud", got, "fraud")
	}
	if !l.IsBanned("total fraud") {
		t.Error("the plain word isn't banned")
	}
	l.Remove("FRAUD")
	if l.IsBanned("total fraud") {
		t.Error("Remove of a different spelling didn't unban it")
	}
}

// Many Saves at once must never leave a broken file or stray temp files.
func TestConcurrentSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "words.txt")
	l := New("scam")
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.Add(fmt.Sprintf("word%c", 'a'+i))
			if err := l.Save(path); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()

	again, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(again.Words(), l.Words()) {
		t.Errorf("after concurrent saves the file has %v, want %v", again.Words(), l.Words())
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("dir has %d files, want only words.txt (temp files left behind?)", len(entries))
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

// A missing list is created empty; an existing one is loaded as it is.
func TestLoadOrCreate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "banned_words.txt")
	l, created, err := LoadOrCreate(path)
	if err != nil || !created || len(l.Words()) != 0 {
		t.Fatalf("missing file: list %v, created %v, err %v; want an empty list, created", l.Words(), created, err)
	}
	if data, err := os.ReadFile(path); err != nil || len(data) != 0 {
		t.Errorf("created file = %q, %v; want an empty file", data, err)
	}

	os.WriteFile(path, []byte("scam\n"), 0o644)
	l, created, err = LoadOrCreate(path)
	if err != nil || created || !l.IsBanned("a scam") {
		t.Errorf("existing file: created %v, err %v, scam banned %v; want it loaded, not created", created, err, l.IsBanned("a scam"))
	}
}
