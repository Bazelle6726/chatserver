package hub

import (
	"sync"

	"chatserver/internal/chat"
)

// Pool is a group of worker goroutines that prepare messages for the live
// feed: each worker filters banned words, then hands the message on (to the
// hub's Publish).
//
//	Submit ──► jobs channel ──► worker 1 ─┐
//	                       ├──► worker 2 ─┼──► deliver (hub.Publish)
//	                       └──► worker 3 ─┘
type Pool struct {
	jobs    chan chat.Message
	banned  func(text string) bool // e.g. an Admin's IsBanned method
	deliver func(m chat.Message)   // e.g. hub.Publish
	wg      sync.WaitGroup
}

// NewPool makes a pool. queue is how many messages can wait for a worker.
func NewPool(banned func(string) bool, deliver func(chat.Message), queue int) *Pool {
	return &Pool{jobs: make(chan chat.Message, queue), banned: banned, deliver: deliver}
}

// Submit queues a message. If the queue is full it waits (back-pressure).
// Never call it after Stop: sending on a closed channel panics.
func (p *Pool) Submit(m chat.Message) {
	p.jobs <- m
}

// Start launches n workers.
//
// TODO(44): n times:
//   - p.wg.Add(1) BEFORE starting the goroutine (not inside it)
//   - go func() { defer p.wg.Done(); ... }()
//   - inside: for m := range p.jobs { ... } - the loop ends when jobs is closed
//   - if p.banned(m.Text), set m.Text = "[removed]"
//   - then p.deliver(m)
//
// m is a COPY of the message (Message is a struct value), so changing m.Text
// doesn't change the original that's stored in the room's history.
// (With Go 1.25+ in go.mod you could use p.wg.Go(func() { ... }) instead.)
func (p *Pool) Start(n int) {
}

// Stop shuts the pool down after every queued message has been delivered.
//
// TODO(45): two steps, in this order:
//  1. close(p.jobs) - no more work; each worker's range loop will end
//     once the queue is empty
//  2. p.wg.Wait()  - wait until every worker has finished
func (p *Pool) Stop() {
}
