package bot

import "sync"

// Pool is a bounded worker pool: at most maxConcurrent messages process at
// once, while each chat's messages run strictly in submission order (a
// dedicated serial worker per chat id — the per-chat ordering contract).
type Pool struct {
	sem   chan struct{}
	mu    sync.Mutex
	chats map[int64]chan func()
}

// NewPool builds a pool processing at most n messages concurrently.
func NewPool(n int) *Pool {
	if n < 1 {
		n = 1
	}
	return &Pool{sem: make(chan struct{}, n), chats: map[int64]chan func(){}}
}

// Submit enqueues fn for chatID. It returns once the work is queued (never
// blocks on other chats' in-flight work beyond the per-chat queue).
func (p *Pool) Submit(chatID int64, fn func()) {
	q := p.queue(chatID)
	q <- fn
}

func (p *Pool) queue(chatID int64) chan func() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if q, ok := p.chats[chatID]; ok {
		return q
	}
	q := make(chan func(), 128)
	p.chats[chatID] = q
	go p.worker(q)
	return q
}

func (p *Pool) worker(q chan func()) {
	for fn := range q {
		p.sem <- struct{}{} // global concurrency cap
		fn()
		<-p.sem
	}
}
