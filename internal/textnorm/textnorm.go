// Package textnorm folds text so that strings which LOOK the same compare
// equal. "Scam", "ｓｃａｍ" (fullwidth), "sсam" (with a Cyrillic с) and
// "sc​am" (with a zero-width space) all fold to "scam".
//
// Moderation uses it so banned words can't slip through in disguise, and the
// store uses it so nobody can register a name that looks like someone else's.
package textnorm

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// confusables maps lowercase letters from other scripts to the Latin letter
// they look like. It covers the common Cyrillic and Greek look-alikes,
// including letters whose UPPERCASE form is the look-alike (Cyrillic В, Н, М
// look like B, H, M), since Skeleton lowercases before looking letters up.
// The complete list is Unicode's confusables.txt (Technical Standard #39).
var confusables = map[rune]rune{
	// Cyrillic
	'а': 'a', 'в': 'b', 'е': 'e', 'ё': 'e', 'з': '3', 'һ': 'h', 'н': 'h',
	'і': 'i', 'ї': 'i', 'ј': 'j', 'к': 'k', 'ӏ': 'l', 'м': 'm', 'о': 'o',
	'р': 'p', 'ԛ': 'q', 'с': 'c', 'ѕ': 's', 'т': 't', 'у': 'y', 'ԝ': 'w',
	'х': 'x', 'ԁ': 'd', 'ь': 'b',
	// Greek
	'α': 'a', 'β': 'b', 'ε': 'e', 'ζ': 'z', 'η': 'n', 'ι': 'i', 'κ': 'k',
	'μ': 'u', 'ν': 'v', 'ο': 'o', 'ρ': 'p', 'τ': 't', 'υ': 'u', 'χ': 'x',
	'ϲ': 'c', 'ϳ': 'j',
	// Latin letters that pass for others
	'ı': 'i', 'ł': 'l', 'ſ': 's',
}

// Skeleton returns the folded form of s:
//
//  1. NFKC normalization turns compatibility forms into plain ones:
//     "ｓ" (fullwidth) and "𝐬" (math bold) become "s", "ﬁ" becomes "fi"
//  2. invisible formatting characters are dropped: zero-width spaces and
//     joiners, soft hyphens, direction marks (Unicode category Cf)
//  3. everything is lowercased
//  4. common Cyrillic and Greek look-alikes become the Latin letter
//
// Two strings with the same skeleton are, for practical purposes,
// indistinguishable on screen.
func Skeleton(s string) string {
	s = norm.NFKC.String(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.Is(unicode.Cf, r) {
			continue
		}
		r = unicode.ToLower(r)
		if latin, ok := confusables[r]; ok {
			r = latin
		}
		b.WriteRune(r)
	}
	return b.String()
}

// lookalikeScripts are the scripts whose letters are easily mistaken for one
// another. Mixing them in one name is almost always a disguise ("pаypal" with
// a Cyrillic а); other scripts, like Japanese mixing kana and kanji, are fine.
var lookalikeScripts = []*unicode.RangeTable{unicode.Latin, unicode.Cyrillic, unicode.Greek}

// MixedScripts reports whether s has letters from more than one of Latin,
// Cyrillic and Greek.
func MixedScripts(s string) bool {
	seen := -1
	for _, r := range s {
		for i, script := range lookalikeScripts {
			if unicode.Is(script, r) {
				if seen != -1 && seen != i {
					return true
				}
				seen = i
			}
		}
	}
	return false
}
