// Package moderation holds the list of banned words for the live feed.
package moderation

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"chatserver/internal/textnorm"
)

// maxWordLen bounds how many single letters IsBanned will glue back together
// ("s c a m" -> "scam"). No banned word is longer than this.
const maxWordLen = 50

// List is a set of banned words, safe to use from many goroutines: the
// pool's workers read it while an admin might be changing it.
type List struct {
	mu    sync.RWMutex
	words map[string]struct{} // folded (see fold); struct{} = "only the key matters"

	saveMu sync.Mutex // one Save at a time, so two admins can't interleave writes
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

// LoadOrCreate is Load, except that a missing file is created empty (and the
// list starts empty) instead of being an error. The server uses it, so a
// fresh checkout starts with a blank list for an admin to fill in. created
// reports whether the file was made.
func LoadOrCreate(path string) (l *List, created bool, err error) {
	l, err = Load(path)
	if err == nil || !errors.Is(err, fs.ErrNotExist) {
		return l, false, err
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		return nil, false, fmt.Errorf("creating banned words file: %w", err)
	}
	return New(), true, nil
}

// leetDigits undoes the usual digit swaps: "sc4m" -> "scam", "fr4ud" -> "fraud".
var leetDigits = map[rune]rune{'0': 'o', '1': 'i', '3': 'e', '4': 'a', '5': 's', '7': 't'}

// leetSymbols undoes symbol swaps: "$cam" -> "scam", "sc@m" -> "scam".
var leetSymbols = map[rune]rune{'@': 'a', '$': 's'}

// fold turns text into the form banned words are stored and compared in:
// textnorm.Skeleton (case, fullwidth, look-alike letters, zero-width
// characters), then stray combining marks dropped ("s̲c̲a̲m̲" -> "scam"), then
// leetspeak digits, and symbols too if symbols is true.
func fold(text string, symbols bool) string {
	var b strings.Builder
	for _, r := range textnorm.Skeleton(text) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if l, ok := leetDigits[r]; ok {
			r = l
		} else if l, ok := leetSymbols[r]; ok && symbols {
			r = l
		}
		b.WriteRune(r)
	}
	return b.String()
}

// tokens splits text into words: runs of letters and digits.
func tokens(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// IsBanned reports whether text contains a banned word, as a whole word and
// ignoring disguises: with "scam" banned, "SCAM!", "sc4m", "$cam", "ѕсаm"
// (Cyrillic), "s c a m" and "s.c.a.m" are all caught, but "scampi" isn't.
//
// Text is checked twice, with and without symbol leetspeak, because a symbol
// can be either a disguised letter ("$cam") or a real separator
// ("bob@scam.com" must still split into "bob", "scam", "com").
func (l *List) IsBanned(text string) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if len(l.words) == 0 {
		return false
	}
	return l.anyBanned(tokens(fold(text, false))) || l.anyBanned(tokens(fold(text, true)))
}

// anyBanned checks each word, and also glues runs of single letters back
// together, so "s c a m" is checked as "scam" (and "a s c a m" finds "scam"
// inside "ascam"). l.mu must be held.
func (l *List) anyBanned(words []string) bool {
	for i, w := range words {
		if _, ok := l.words[w]; ok {
			return true
		}
		if !isSingle(w) {
			continue
		}
		var glued strings.Builder
		glued.WriteString(w)
		for j := i + 1; j < len(words) && j-i < maxWordLen && isSingle(words[j]); j++ {
			glued.WriteString(words[j])
			if _, ok := l.words[glued.String()]; ok {
				return true
			}
		}
	}
	return false
}

// isSingle reports whether w is exactly one character.
func isSingle(w string) bool {
	return utf8.RuneCountInString(w) == 1
}

// Add bans a word while the server runs, and returns it in the form it's
// stored and listed in. Words are folded like messages are, so adding "Sc4m"
// bans "scam" and every disguise of it.
func (l *List) Add(word string) string {
	word = fold(strings.TrimSpace(word), true)
	if word == "" {
		return ""
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.words[word] = struct{}{}
	return word
}

// Remove unbans a word. It's folded first, so Remove("SCAM") undoes Add("scam").
func (l *List) Remove(word string) {
	word = fold(strings.TrimSpace(word), true)
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.words, word)
}

// Words returns the banned words, sorted.
func (l *List) Words() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return slices.Sorted(maps.Keys(l.words))
}

// Save writes the list to path, one word per line, sorted. Comments in the
// original file are not kept.
//
// It's safe to call from many goroutines at once, and a crash part-way never
// leaves a half-written file: the list goes to a temporary file next to path,
// which then replaces path in one step (os.Rename).
func (l *List) Save(path string) error {
	l.saveMu.Lock()
	defer l.saveMu.Unlock()

	// Take the snapshot while holding saveMu, so the last Save to finish
	// always writes the newest list.
	data := strings.Join(l.Words(), "\n") + "\n"

	tmp, err := os.CreateTemp(filepath.Dir(path), ".banned-words-*.tmp")
	if err != nil {
		return fmt.Errorf("saving banned words: %w", err)
	}
	defer os.Remove(tmp.Name()) // a no-op once the rename has happened
	if _, err := tmp.WriteString(data); err != nil {
		tmp.Close()
		return fmt.Errorf("saving banned words: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("saving banned words: %w", err)
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return fmt.Errorf("saving banned words: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("saving banned words: %w", err)
	}
	return nil
}
