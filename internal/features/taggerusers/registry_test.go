package taggerusers

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/reijo1337/ToxicBot/internal/domain/chat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	groupA = int64(-1001)
	groupB = int64(-1002)
)

var errDisk = errors.New("disk I/O error")

type fakeStore struct {
	loadErr   error
	writeErr  error
	loaded    []chat.TaggerUser
	upserts   []chat.TaggerUser
	deletes   []memberKey
	chatsGone []int64
}

func (s *fakeStore) LoadAll(context.Context) ([]chat.TaggerUser, error) {
	return s.loaded, s.loadErr
}

func (s *fakeStore) Upsert(_ context.Context, u chat.TaggerUser) error {
	if s.writeErr != nil {
		return s.writeErr
	}
	s.upserts = append(s.upserts, u)
	return nil
}

func (s *fakeStore) Delete(_ context.Context, chatID, userID int64) error {
	if s.writeErr != nil {
		return s.writeErr
	}
	s.deletes = append(s.deletes, memberKey{chatID: chatID, userID: userID})
	return nil
}

func (s *fakeStore) DeleteChat(_ context.Context, chatID int64) error {
	if s.writeErr != nil {
		return s.writeErr
	}
	s.chatsGone = append(s.chatsGone, chatID)
	return nil
}

func newRegistry(t *testing.T, store *fakeStore) *Registry {
	t.Helper()
	r, err := New(t.Context(), store)
	require.NoError(t, err)
	return r
}

func sorted(ids []int64) []int64 {
	slices.Sort(ids)
	return ids
}

// spec: TAG-022
func TestNew_FailsWhenStoreUnavailable(t *testing.T) {
	t.Parallel()

	r, err := New(t.Context(), &fakeStore{loadErr: errDisk})

	require.ErrorIs(t, err, errDisk)
	assert.Nil(t, r)
}

// spec: TAG-014
func TestNew_RestoresStoredUsers(t *testing.T) {
	t.Parallel()

	r := newRegistry(t, &fakeStore{loaded: []chat.TaggerUser{
		{ChatID: groupA, UserID: 1, Author: "@alice"},
		{ChatID: groupA, UserID: 2, Author: "@bob"},
		{ChatID: groupB, UserID: 1, Author: "@alice"},
	}})

	assert.Equal(t, []int64{groupB, groupA}, sorted(r.Chats()))
	assert.Equal(t, []int64{1, 2}, sorted(r.Users(groupA)))
	author, ok := r.Author(groupA, 2)
	assert.True(t, ok)
	assert.Equal(t, "@bob", author)
}

func TestNew_EmptyStoreIsValid(t *testing.T) {
	t.Parallel()

	r := newRegistry(t, &fakeStore{})

	assert.Empty(t, r.Chats())
	assert.Empty(t, r.Users(groupA))
}

func TestAdd_WritesOnlyNewUserOrChangedAuthor(t *testing.T) {
	t.Parallel()

	store := &fakeStore{}
	r := newRegistry(t, store)
	ctx := t.Context()

	require.NoError(t, r.Add(ctx, chat.TaggerUser{ChatID: groupA, UserID: 1, Author: "@alice"}))
	require.NoError(t, r.Add(ctx, chat.TaggerUser{ChatID: groupA, UserID: 1, Author: "@alice"}))
	require.NoError(t, r.Add(ctx, chat.TaggerUser{ChatID: groupA, UserID: 1, Author: "@alice2"}))

	assert.Equal(t, []chat.TaggerUser{
		{ChatID: groupA, UserID: 1, Author: "@alice"},
		{ChatID: groupA, UserID: 1, Author: "@alice2"},
	}, store.upserts)
	assert.Equal(t, []int64{1}, r.Users(groupA), "без дубликатов")
	author, _ := r.Author(groupA, 1)
	assert.Equal(t, "@alice2", author)
}

// spec: TAG-016
func TestRemove_OnlyThatChatMember(t *testing.T) {
	t.Parallel()

	store := &fakeStore{loaded: []chat.TaggerUser{
		{ChatID: groupA, UserID: 1, Author: "@alice"},
		{ChatID: groupA, UserID: 2, Author: "@bob"},
		{ChatID: groupB, UserID: 1, Author: "@alice"},
	}}
	r := newRegistry(t, store)

	require.NoError(t, r.Remove(t.Context(), groupA, 1))
	require.NoError(t, r.Remove(t.Context(), groupA, 42), "неизвестный участник")

	assert.Equal(t, []memberKey{{chatID: groupA, userID: 1}}, store.deletes)
	assert.Equal(t, []int64{2}, r.Users(groupA))
	assert.Equal(t, []int64{1}, r.Users(groupB))
	_, ok := r.Author(groupA, 1)
	assert.False(t, ok)

	require.NoError(t, r.Remove(t.Context(), groupA, 2))
	assert.Equal(t, []int64{groupB}, r.Chats(), "опустевший чат пропадает")
}

// spec: TAG-018
func TestRemoveChat_ForgetsWholeChat(t *testing.T) {
	t.Parallel()

	store := &fakeStore{loaded: []chat.TaggerUser{
		{ChatID: groupA, UserID: 1, Author: "@alice"},
		{ChatID: groupA, UserID: 2, Author: "@bob"},
		{ChatID: groupB, UserID: 1, Author: "@alice"},
	}}
	r := newRegistry(t, store)

	require.NoError(t, r.RemoveChat(t.Context(), groupA))

	assert.Equal(t, []int64{groupA}, store.chatsGone)
	assert.Empty(t, r.Users(groupA))
	_, ok := r.Author(groupA, 2)
	assert.False(t, ok)
	assert.Equal(t, []int64{groupB}, r.Chats())
}

// spec: TAG-021
func TestWriteFailureKeepsPreviousList(t *testing.T) {
	t.Parallel()

	store := &fakeStore{loaded: []chat.TaggerUser{{ChatID: groupA, UserID: 1, Author: "@alice"}}}
	r := newRegistry(t, store)
	store.writeErr = errDisk
	ctx := t.Context()

	require.ErrorIs(
		t,
		r.Add(ctx, chat.TaggerUser{ChatID: groupA, UserID: 2, Author: "@bob"}),
		errDisk,
	)
	require.ErrorIs(
		t,
		r.Add(ctx, chat.TaggerUser{ChatID: groupA, UserID: 1, Author: "@new"}),
		errDisk,
	)
	require.ErrorIs(t, r.Remove(ctx, groupA, 1), errDisk)
	require.ErrorIs(t, r.RemoveChat(ctx, groupA), errDisk)

	assert.Equal(t, []int64{groupA}, r.Chats())
	assert.Equal(t, []int64{1}, r.Users(groupA))
	author, _ := r.Author(groupA, 1)
	assert.Equal(t, "@alice", author)
}

func TestUsers_ReturnsCopy(t *testing.T) {
	t.Parallel()

	r := newRegistry(
		t,
		&fakeStore{loaded: []chat.TaggerUser{{ChatID: groupA, UserID: 1, Author: "@a"}}},
	)

	users := r.Users(groupA)
	users[0] = 99

	assert.Equal(t, []int64{1}, r.Users(groupA))
}
