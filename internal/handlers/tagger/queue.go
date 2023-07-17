package tagger

import (
	"sync"
	"time"
)

type taggerJob struct {
	tagAt  time.Time
	chatID string
}

type taggerQueue struct {
	queue []taggerJob
	mu    sync.Mutex
}

func (t *taggerQueue) clean() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.queue = t.queue[:0]
}

// Len is the number of elements in the collection.
func (t *taggerQueue) Len() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.queue)
}

// Less reports whether the element with index i
// must sort before the element with index j.
func (t *taggerQueue) Less(i, j int) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.queue[i].tagAt.Before(t.queue[j].tagAt)
}

// Swap swaps the elements with indexes i and j.
func (t *taggerQueue) Swap(i, j int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.queue[i], t.queue[j] = t.queue[j], t.queue[i]
}

// Push add x as element Len()
func (t *taggerQueue) Push(x any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.queue = append(t.queue, x.(taggerJob))
}

// Pop remove and return element Len() - 1
func (t *taggerQueue) Pop() any {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := t.queue[len(t.queue)-1]
	t.queue = t.queue[:len(t.queue)-1]
	return out
}
