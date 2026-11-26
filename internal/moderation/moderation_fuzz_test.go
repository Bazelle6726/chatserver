package moderation

import (
	"strings"
	"testing"
)

// If an admin's Add accepts a word, then posting exactly that word must be
// caught. Otherwise the admin panel shows the word as banned while it never
// matches anything.
//
//	go test ./internal/moderation -run XXX -fuzz FuzzAddedWordIsBanned -fuzztime 30s
func FuzzAddedWordIsBanned(f *testing.F) {
	for _, s := range []string{"scam", "Sc4m", "$cam", "fraud", "ѕсаm", "s c a m"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, word string) {
		l := New()
		stored := l.Add(word)
		if stored == "" {
			t.Skip() // rejected: nothing was banned, nothing to check
		}
		if !l.IsBanned(word) {
			t.Errorf("Add(%+q) stored %+q, but IsBanned(%+q) is false", word, stored, word)
		}
	})
}

// A banned word must still be caught inside a sentence and in disguise.
//
//	go test ./internal/moderation -run XXX -fuzz FuzzDisguisedWordIsBanned -fuzztime 30s
func FuzzDisguisedWordIsBanned(f *testing.F) {
	f.Add("scam", "cheap", "here", uint8(0))
	f.Add("fraud", "", "!", uint8(1))
	f.Fuzz(func(t *testing.T, word, before, after string, how uint8) {
		// Only plain lowercase ASCII words: the property is about disguises,
		// not about which words are allowed on the list.
		if len(word) < 3 || len(word) > 20 || strings.Trim(word, "abcdefghijklmnopqrstuvwxyz") != "" {
			t.Skip()
		}
		// Keep the banned word a separate word: "scampi" is allowed on purpose.
		if !strings.HasSuffix(before, " ") {
			before += " "
		}
		if !strings.HasPrefix(after, " ") {
			after = " " + after
		}
		var disguised string
		switch how % 3 {
		case 0:
			disguised = strings.ToUpper(word)
		case 1:
			disguised = strings.Join(strings.Split(word, ""), "​")
		case 2:
			disguised = strings.NewReplacer("a", "а", "c", "с", "e", "е", "o", "о", "p", "р").Replace(word) // Cyrillic
		}
		l := New(word)
		text := before + disguised + after
		if !l.IsBanned(text) {
			t.Errorf("with %q banned, %+q was not caught", word, text)
		}
	})
}
