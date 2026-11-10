// Package moderation holds the list of banned words for the live feed.
package moderation

import (
	"bufio"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"
	"unicode"
)

// List is a set of banned words, safe to use from many goroutines: the
// pool's workers read it while an admin might be changing it.
type List struct {
	mu    sync.RWMutex
	words map[string]struct{} // lowercase; struct{} = "only the key matters"
}

// New returns a list holding words (handy in tests).
func New(words ...string) *List {
	l := &List{words: make(map[string]struct{})}
	for _, w := range words {
		l.Add(w)
	}
	return l
}

// Load reads one word per line from path, skipping blank lines and lines
// that start with #.
func Load(path string) (*List, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("loading banned words: %w", err)
	}
	defer f.Close()

	l := New()
	lines := bufio.NewScanner(f)
	for lines.Scan() {
		line := strings.TrimSpace(lines.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		l.Add(line)
	}
	if err := lines.Err(); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return l, nil
}

// IsBanned reports whether text contains a banned word, as a whole word and
// ignoring case: with "scam" banned, "SCAM!" is caught but "scampi" isn't.
func (l *List) IsBanned(text string) bool {
	// Split on anything that isn't a letter or digit, so punctuation and
	// spaces separate words: "hello, scam!" -> ["hello", "scam"].
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	l.mu.RLock()
	defer l.mu.RUnlock()
	for _, w := range words {
		if _, ok := l.words[w]; ok {
			return true
		}
	}
	return false
}

// Add bans a word while the server runs.
func (l *List) Add(word string) {
	word = strings.ToLower(strings.TrimSpace(word))
	if word == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.words[word] = struct{}{}
}

// Remove unbans a word.
func (l *List) Remove(word string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.words, strings.ToLower(strings.TrimSpace(word)))
}

// Words returns the banned words, sorted.
func (l *List) Words() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return slices.Sorted(maps.Keys(l.words))
}

// Save writes the list to path, one word per line, sorted. Comments in the
// original file are not kept.
func (l *List) Save(path string) error {
	data := strings.Join(l.Words(), "\n") + "\n"
	return os.WriteFile(path, []byte(data), 0o644)
}
