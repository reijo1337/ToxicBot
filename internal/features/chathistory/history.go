package chathistory

import (
	"context"
	"slices"
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
	b.persistLocked(context.Background(), chatID)
}

func (b *Buffer) AddReaction(ctx context.Context, chatID int64, e Entry) error {
	if e.Reaction == nil {
		return nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if err := b.loadLocked(ctx, chatID); err != nil {
		return err
	}

	for _, entry := range b.data[chatID] {
		if entry.Reaction != nil && entry.Reaction.UpdateID == e.Reaction.UpdateID {
			return nil
		}
	}

	e = cloneEntry(e)
	reaction := e.Reaction.ResolveTarget(b.data[chatID])
	e.Reaction = &reaction
	b.appendLocked(chatID, e)
	b.persistLocked(ctx, chatID)

	return nil
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

	b.persistLocked(context.Background(), chatID)
}

func (b *Buffer) Get(chatID int64) []Entry {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.ensureLoadedLocked(chatID)

	return cloneEntries(b.data[chatID])
}

func (b *Buffer) appendLocked(chatID int64, e Entry) {
	buf := b.data[chatID]
	buf = append(buf, cloneEntry(e))
	if len(buf) > b.maxSize {
		buf = buf[len(buf)-b.maxSize:]
	}

	b.data[chatID] = buf
}

func (b *Buffer) ensureLoadedLocked(chatID int64) {
	ctx := context.Background()
	if err := b.loadLocked(ctx, chatID); err != nil {
		b.log.Warn(b.log.WithError(ctx, err), "chathistory: failed to load history")
	}
}

func (b *Buffer) loadLocked(ctx context.Context, chatID int64) error {
	if b.loaded[chatID] {
		return nil
	}

	entries, err := b.store.Load(ctx, chatID)
	if err != nil {
		return err
	}

	b.loaded[chatID] = true

	if len(entries) == 0 {
		return nil
	}

	if len(entries) > b.maxSize {
		entries = entries[len(entries)-b.maxSize:]
	}

	b.data[chatID] = cloneEntries(entries)

	return nil
}

func (b *Buffer) persistLocked(ctx context.Context, chatID int64) {
	if err := b.store.Save(ctx, chatID, cloneEntries(b.data[chatID])); err != nil {
		b.log.Warn(b.log.WithError(ctx, err), "chathistory: failed to save history")
	}
}

func cloneEntries(entries []Entry) []Entry {
	cloned := make([]Entry, len(entries))

	for i, entry := range entries {
		cloned[i] = cloneEntry(entry)
	}

	return cloned
}

func cloneEntry(entry Entry) Entry {
	if entry.Reaction != nil {
		reaction := *entry.Reaction
		reaction.Added = slices.Clone(reaction.Added)
		reaction.Removed = slices.Clone(reaction.Removed)
		entry.Reaction = &reaction
	}

	return entry
}

type noopStore struct{}

func (noopStore) Load(_ context.Context, _ int64) ([]Entry, error) { return nil, nil }
func (noopStore) Save(_ context.Context, _ int64, _ []Entry) error { return nil }

type noopLogger struct{}

func (noopLogger) Warn(_ context.Context, _ string)                       {}
func (noopLogger) WithError(ctx context.Context, _ error) context.Context { return ctx }
