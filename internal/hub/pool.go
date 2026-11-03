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
func (p *Pool) Start(n int) {
	for i := 0; i < n; i++ {
		p.wg.Add(1)
		go func() { // each worker runs this loop
			defer p.wg.Done()
			for m := range p.jobs { // each worker gets a message from the jobs channel
				if p.banned(m.Text) {
					m.Text = "[removed]"
				}
				p.deliver(m)
			}
		}()
	}
}

// Stop shuts the pool down after every queued message has been delivered.
func (p *Pool) Stop() {
	close(p.jobs) // no more work; each loop will end once the queue is empty
	p.wg.Wait()   // wait until every worker has finished
}
