package chathistory

import (
	"context"
	"sync"
)

type Store interface {
	Load(ctx context.Context, chatID int64) ([]Entry, error)
	Save(ctx context.Context, chatID int64, entries []Entry) error
}

type Logger interface {
	Warn(ctx context.Context, msg string)
	WithError(ctx context.Context, err error) context.Context
}

type Option func(*Buffer)

func WithStore(s Store) Option {
	return func(b *Buffer) { b.store = s }
}

func WithLogger(l Logger) Option {
	return func(b *Buffer) { b.log = l }
}

type Buffer struct {
	mu      sync.Mutex
	data    map[int64][]Entry
	loaded  map[int64]bool
	store   Store
	log     Logger
	maxSize int
}

func NewBuffer(maxSize int, opts ...Option) *Buffer {
	if maxSize <= 0 {
		panic("chathistory: maxSize must be positive")
	}

	b := &Buffer{
		data:    make(map[int64][]Entry),
		loaded:  make(map[int64]bool),
		store:   noopStore{},
		log:     noopLogger{},
		maxSize: maxSize,
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

func (b *Buffer) Add(chatID int64, e Entry) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.ensureLoadedLocked(chatID)
	b.appendLocked(chatID, e)
	b.persistLocked(chatID)
}

// AddAll appends multiple entries under a single lock, so no concurrent Add
// from another goroutine can interleave between them. Use for atomic
// user→bot pairs.
func (b *Buffer) AddAll(chatID int64, entries ...Entry) {
	if len(entries) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	b.ensureLoadedLocked(chatID)
	for _, e := range entries {
		b.appendLocked(chatID, e)
	}
	b.persistLocked(chatID)
}

func (b *Buffer) Get(chatID int64) []Entry {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.ensureLoadedLocked(chatID)

	src := b.data[chatID]
	out := make([]Entry, len(src))
	copy(out, src)
	return out
}

func (b *Buffer) appendLocked(chatID int64, e Entry) {
	buf := b.data[chatID]
	buf = append(buf, e)
	if len(buf) > b.maxSize {
		buf = buf[len(buf)-b.maxSize:]
	}
	b.data[chatID] = buf
}

func (b *Buffer) ensureLoadedLocked(chatID int64) {
	if b.loaded[chatID] {
		return
	}

	ctx := context.Background()
	entries, err := b.store.Load(ctx, chatID)
	if err != nil {
		// Don't mark as loaded — let the next call retry.
		// Otherwise a transient error would silently overwrite persisted history.
		b.log.Warn(b.log.WithError(ctx, err), "chathistory: failed to load history")
		return
	}
	b.loaded[chatID] = true

	if len(entries) == 0 {
		return
	}
	if len(entries) > b.maxSize {
		entries = entries[len(entries)-b.maxSize:]
	}
	b.data[chatID] = entries
}

func (b *Buffer) persistLocked(chatID int64) {
	ctx := context.Background()
	if err := b.store.Save(ctx, chatID, b.data[chatID]); err != nil {
		b.log.Warn(b.log.WithError(ctx, err), "chathistory: failed to save history")
	}
}

type noopStore struct{}

func (noopStore) Load(_ context.Context, _ int64) ([]Entry, error) { return nil, nil }
func (noopStore) Save(_ context.Context, _ int64, _ []Entry) error { return nil }

type noopLogger struct{}

func (noopLogger) Warn(_ context.Context, _ string)                       {}
func (noopLogger) WithError(ctx context.Context, _ error) context.Context { return ctx }
