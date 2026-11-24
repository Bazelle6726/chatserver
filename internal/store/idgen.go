package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync/atomic"
)

// NewIDGenerator returns a function that produces "prefix-1", "prefix-2", ...
//
// This is a closure: the returned function keeps its own
// counter variable alive between calls, and each generator
// has its own counter.
//
//	next := NewIDGenerator("u")
//	next() // "u-1"
//	next() // "u-2"
//
// The counter is declared, then returns a func() string that increments it
// and returns fmt.Sprintf("%s-%d", prefix, counter).
func NewIDGenerator(prefix string) func() string {
	counter := int64(0)
	return func() string {
		n := atomic.AddInt64(&counter, 1)
		return fmt.Sprintf("%s-%d", prefix, n)
	}
}

// NewRandomIDGenerator returns a function that produces "prefix-" followed by
// 16 random hex digits (64 bits), e.g. "u-3f9c2a1b7e4d5c60".
//
// User IDs are random, not counted, so they don't reveal how many users
// there are and can't be walked one by one (u-1, u-2, ...).
func NewRandomIDGenerator(prefix string) func() string {
	return func() string {
		b := make([]byte, 8)
		rand.Read(b) // never fails (crypto/rand panics instead)
		return prefix + "-" + hex.EncodeToString(b)
	}
}
