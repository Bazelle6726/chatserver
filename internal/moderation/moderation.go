package moderation

import (
	"sync"
)

// List is a set of banned words, safe to use from many goroutines.
type List struct {
	mu    sync.RWMutex
	words map[string]struct{} // lowercase
}

func Load(path string) (*List, error)     // read the file; skip blanks and # comments
func (l *List) IsBanned(text string) bool // whole words, ignoring case
func (l *List) Add(word string)           // change the list while running
func (l *List) Remove(word string)
func (l *List) Save(path string) error // write it back to the file
