package chathistory_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeStore struct {
	mu        sync.Mutex
	loadCalls map[int64]int
	saveCalls map[int64]int
	saved     map[int64][]chathistory.Entry
	preload   map[int64][]chathistory.Entry
	loadErr   error
	saveErr   error
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		loadCalls: map[int64]int{},
		saveCalls: map[int64]int{},
		saved:     map[int64][]chathistory.Entry{},
		preload:   map[int64][]chathistory.Entry{},
	}
}

func (s *fakeStore) Load(_ context.Context, chatID int64) ([]chathistory.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadCalls[chatID]++
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	out := make([]chathistory.Entry, len(s.preload[chatID]))
	copy(out, s.preload[chatID])
	return out, nil
}

func (s *fakeStore) Save(_ context.Context, chatID int64, entries []chathistory.Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveCalls[chatID]++
	if s.saveErr != nil {
		return s.saveErr
	}
	out := make([]chathistory.Entry, len(entries))
	copy(out, entries)
	s.saved[chatID] = out
	return nil
}

func (s *fakeStore) loadCount(chatID int64) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadCalls[chatID]
}

func (s *fakeStore) saveCount(chatID int64) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveCalls[chatID]
}

func (s *fakeStore) lastSaved(chatID int64) []chathistory.Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]chathistory.Entry, len(s.saved[chatID]))
	copy(out, s.saved[chatID])
	return out
}

func (s *fakeStore) setLoadErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadErr = err
}

type recordingLogger struct {
	mu    sync.Mutex
	warns int
}

func (l *recordingLogger) Warn(_ context.Context, _ string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warns++
}

func (l *recordingLogger) WithError(ctx context.Context, _ error) context.Context {
	return ctx
}

func (l *recordingLogger) warnCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.warns
}

func TestBuffer_Get_LoadsFromStoreOnce(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.preload[7] = []chathistory.Entry{
		{ID: 1, Author: "@a", Text: "old1"},
		{ID: 2, Author: "@b", Text: "old2"},
	}
	buf := chathistory.NewBuffer(50, chathistory.WithStore(store))

	got1 := buf.Get(7)
	got2 := buf.Get(7)

	require.Len(t, got1, 2)
	assert.Equal(t, "old1", got1[0].Text)
	assert.Equal(t, got1, got2)
	assert.Equal(t, 1, store.loadCount(7), "store.Load must be called once and cached")
}

func TestBuffer_Add_LazyLoadsThenAppendsAndSaves(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.preload[7] = []chathistory.Entry{{ID: 1, Text: "old"}}
	buf := chathistory.NewBuffer(50, chathistory.WithStore(store))

	buf.Add(7, chathistory.Entry{ID: 2, Text: "new"})

	history := buf.Get(7)
	require.Len(t, history, 2)
	assert.Equal(t, "old", history[0].Text)
	assert.Equal(t, "new", history[1].Text)

	assert.Equal(t, 1, store.loadCount(7))
	assert.Equal(t, 1, store.saveCount(7))
	assert.Equal(t, history, store.lastSaved(7))
}

func TestBuffer_AddAll_SavesOncePerCall(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	buf := chathistory.NewBuffer(50, chathistory.WithStore(store))

	buf.AddAll(7,
		chathistory.Entry{ID: 1, Text: "a"},
		chathistory.Entry{ID: 2, Text: "b"},
		chathistory.Entry{ID: 3, Text: "c"},
	)

	assert.Equal(t, 1, store.saveCount(7))
	assert.Equal(t, buf.Get(7), store.lastSaved(7))
}

func TestBuffer_Save_ReceivesTrimmedSliceOnOverflow(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	buf := chathistory.NewBuffer(2, chathistory.WithStore(store))

	buf.Add(1, chathistory.Entry{ID: 1, Text: "a"})
	buf.Add(1, chathistory.Entry{ID: 2, Text: "b"})
	buf.Add(1, chathistory.Entry{ID: 3, Text: "c"})

	saved := store.lastSaved(1)
	require.Len(t, saved, 2)
	assert.Equal(t, "b", saved[0].Text)
	assert.Equal(t, "c", saved[1].Text)
}

func TestBuffer_Load_ErrorIsLoggedAndBufferStaysEmpty(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.loadErr = errors.New("boom")
	log := &recordingLogger{}
	buf := chathistory.NewBuffer(50, chathistory.WithStore(store), chathistory.WithLogger(log))

	assert.Empty(t, buf.Get(7))
	assert.GreaterOrEqual(t, log.warnCount(), 1)
}

func TestBuffer_Load_ErrorRetriesOnNextCallAndPreservesHistory(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.preload[7] = []chathistory.Entry{
		{ID: 1, Author: "@a", Text: "old1"},
		{ID: 2, Author: "@b", Text: "old2"},
	}
	store.setLoadErr(errors.New("boom"))
	log := &recordingLogger{}
	buf := chathistory.NewBuffer(50, chathistory.WithStore(store), chathistory.WithLogger(log))

	// First call: Load fails. Buffer must NOT mark the chat as loaded —
	// otherwise the next Add would overwrite the persisted history with
	// an empty/short slice.
	assert.Empty(t, buf.Get(7))
	assert.Equal(t, 1, store.loadCount(7))

	// Storage recovers.
	store.setLoadErr(nil)

	// Next call must retry Load and return the persisted history.
	got := buf.Get(7)
	require.Len(t, got, 2)
	assert.Equal(t, "old1", got[0].Text)
	assert.Equal(t, "old2", got[1].Text)
	assert.Equal(t, 2, store.loadCount(7), "Load must be retried after a previous failure")
	assert.GreaterOrEqual(t, log.warnCount(), 1)
}

func TestBuffer_Add_AfterLoadFailureRecoversPersistedHistoryOnceStoreHeals(t *testing.T) {
	t.Parallel()

	const chatID int64 = 42
	store := newFakeStore()
	store.preload[chatID] = []chathistory.Entry{
		{ID: 1, Text: "persisted"},
	}
	store.setLoadErr(errors.New("boom"))
	log := &recordingLogger{}
	buf := chathistory.NewBuffer(50, chathistory.WithStore(store), chathistory.WithLogger(log))

	// Trigger a failed Load — buffer must NOT mark the chat as loaded.
	assert.Empty(t, buf.Get(chatID))
	assert.Equal(t, 1, store.loadCount(chatID))

	// Storage recovers.
	store.setLoadErr(nil)

	// Add must retry Load, see the persisted entry, append the new one,
	// and save the merged slice — not overwrite history with just {new}.
	buf.Add(chatID, chathistory.Entry{ID: 2, Text: "new"})

	saved := store.lastSaved(chatID)
	require.Len(t, saved, 2, "must merge persisted history with the new entry, not overwrite it")
	assert.Equal(t, "persisted", saved[0].Text)
	assert.Equal(t, "new", saved[1].Text)
	assert.Equal(t, 2, store.loadCount(chatID), "Load must be retried after a previous failure")
}

func TestBuffer_Save_ErrorDoesNotLoseInMemoryState(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.saveErr = errors.New("disk full")
	log := &recordingLogger{}
	buf := chathistory.NewBuffer(50, chathistory.WithStore(store), chathistory.WithLogger(log))

	buf.Add(1, chathistory.Entry{ID: 1, Text: "x"})

	history := buf.Get(1)
	require.Len(t, history, 1)
	assert.Equal(t, "x", history[0].Text)
	assert.GreaterOrEqual(t, log.warnCount(), 1)
}

func TestBuffer_NoStore_BackwardCompatible(t *testing.T) {
	t.Parallel()

	buf := chathistory.NewBuffer(50)
	buf.Add(1, chathistory.Entry{ID: 1, Text: "x"})
	assert.Equal(t, "x", buf.Get(1)[0].Text)
}
