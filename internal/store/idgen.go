package store

import "fmt"

// NewIDGenerator returns a function that produces "prefix-1", "prefix-2", ...
//
// This is a closure: the returned function keeps its own
// counter variable alive between calls, and each generator has its own counter.
//
//	next := NewIDGenerator("u")
//	next() // "u-1"
//	next() // "u-2"
//
// TODO(15): declare a counter, then return a func() string that increments it
// and returns fmt.Sprintf("%s-%d", prefix, counter).
func NewIDGenerator(prefix string) func() string {
	_ = fmt.Sprintf // remove this line once you use fmt
	return func() string { return "" }
}
