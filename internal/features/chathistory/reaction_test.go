package chathistory_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/reijo1337/ToxicBot/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	changedReactionInput    = "changed input"
	changedReactionSnapshot = "changed snapshot"
	changedStoreLoad        = "changed load"
	changedStoreSave        = "changed save"
)

// spec: HIST-026
func TestBuffer_AddReaction_StoresEventAndCapturesTargetAuthorship(t *testing.T) {
	t.Parallel()

	for name, target := range map[string]chathistory.Entry{
		"user": {
			ID:       10,
			SenderID: 101,
			Author:   "alice",
			Text:     "original",
		},
		"bot": {
			ID:      10,
			Author:  "bot",
			Text:    "bot reply",
			FromBot: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			buf := chathistory.NewBuffer(10)
			buf.Add(1, target)
			buf.Add(2, chathistory.Entry{
				ID:       10,
				SenderID: 999,
				Author:   "other chat",
			})
			stamp := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
			event := chathistory.Entry{
				Time: stamp,
				Reaction: &chathistory.Reaction{
					UpdateID:  70,
					MessageID: 10,
					ActorID:   202,
					Actor:     "bob",
					Added: []chathistory.ReactionType{
						{
							Type:  "emoji",
							Value: "👍",
						},
					},
					Removed: []chathistory.ReactionType{
						{
							Type:  "custom_emoji",
							Value: "123",
						},
					},
				},
			}

			require.NoError(t, buf.AddReaction(context.Background(), 1, event))

			got := buf.Get(1)

			require.Len(t, got, 2)
			assert.Equal(t, chathistory.Entry{
				Time: stamp,
				Reaction: &chathistory.Reaction{
					UpdateID:  70,
					MessageID: 10,
					ActorID:   202,
					Actor:     "bob",
					Added: []chathistory.ReactionType{
						{
							Type:  "emoji",
							Value: "👍",
						},
					},
					Removed: []chathistory.ReactionType{
						{
							Type:  "custom_emoji",
							Value: "123",
						},
					},
					TargetKnown:    true,
					TargetSenderID: target.SenderID,
					TargetAuthor:   target.Author,
					TargetFromBot:  target.FromBot,
				},
			}, got[1])
			assert.False(
				t,
				event.Reaction.TargetKnown,
				"capturing a target must not mutate the caller",
			)
			assert.Equal(t, []chathistory.Entry{
				{
					ID:       10,
					SenderID: 999,
					Author:   "other chat",
				},
			}, buf.Get(2))
		})
	}
}

// spec: HIST-027
func TestBuffer_AddReaction_SharesWindowWithMessages(t *testing.T) {
	t.Parallel()

	buf := chathistory.NewBuffer(3)
	buf.Add(1, chathistory.Entry{
		ID:       1,
		SenderID: 101,
		Author:   "alice",
		Text:     "evicted target",
	})

	require.NoError(t, buf.AddReaction(context.Background(), 1, reactionEntry(10, 1)))

	buf.Add(1, chathistory.Entry{
		ID:   2,
		Text: "second",
	})

	require.NoError(t, buf.AddReaction(context.Background(), 1, reactionEntry(11, 1)))
	require.NoError(t, buf.AddReaction(context.Background(), 1, reactionEntry(12, 1)))

	got := buf.Get(1)

	require.Len(t, got, 3)
	assert.Equal(t, 2, got[0].ID)
	require.NotNil(t, got[1].Reaction)
	assert.Equal(t, 11, got[1].Reaction.UpdateID)
	assert.True(t, got[1].Reaction.TargetKnown)
	assert.Equal(t, "alice", got[1].Reaction.TargetAuthor)
	require.NotNil(t, got[2].Reaction)
	assert.Equal(t, 12, got[2].Reaction.UpdateID)
	assert.False(t, got[2].Reaction.TargetKnown, "an evicted target must remain unknown")
}

// spec: HIST-030
func TestBuffer_ReactionSnapshotsAreIndependent(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"reaction", "add", "add_all"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			buf := chathistory.NewBuffer(10)
			entry := reactionEntry(70, 10)

			switch path {
			case "reaction":
				require.NoError(t, buf.AddReaction(context.Background(), 1, entry))
			case "add":
				buf.Add(1, entry)
			case "add_all":
				buf.AddAll(1, entry)
			}

			entry.Reaction.Actor = changedReactionInput
			entry.Reaction.Added[0].Value = changedReactionInput
			entry.Reaction.Removed[0].Value = changedReactionInput
			first := buf.Get(1)
			second := buf.Get(1)

			require.Len(t, first, 1)
			require.Equal(t, reactionEntry(70, 10), first[0])

			first[0].Reaction.Actor = changedReactionSnapshot
			first[0].Reaction.Added[0].Type = changedReactionSnapshot
			first[0].Reaction.Removed[0].Value = changedReactionSnapshot

			assert.Equal(t, []chathistory.Entry{reactionEntry(70, 10)}, second)
			assert.Equal(t, second, buf.Get(1))
		})
	}
}

// spec: HIST-030
func TestBuffer_ReactionStoreBoundariesAreIndependent(t *testing.T) {
	t.Parallel()

	store := &retainingReactionStore{
		loaded: []chathistory.Entry{reactionEntry(70, 10)},
	}

	buf := chathistory.NewBuffer(10, chathistory.WithStore(store))
	original := buf.Get(1)
	store.loaded[0].Reaction.Actor = changedStoreLoad
	store.loaded[0].Reaction.Added[0].Value = changedStoreLoad
	store.loaded[0].Reaction.Removed[0].Value = changedStoreLoad
	store.loaded[0].Text = changedStoreLoad

	require.Equal(t, []chathistory.Entry{reactionEntry(70, 10)}, buf.Get(1))

	require.NoError(t, buf.AddReaction(context.Background(), 1, reactionEntry(71, 11)))

	firstSave := store.saved

	require.Len(t, firstSave, 2)

	buf.Add(1, chathistory.Entry{
		ID:   20,
		Text: "next message",
	})
	store.saved[0].Text = changedStoreSave
	store.saved[0].Reaction.Actor = changedStoreSave
	store.saved[0].Reaction.Added[0].Value = changedStoreSave
	store.saved[0].Reaction.Removed[0].Value = changedStoreSave
	store.saved[1].Reaction.Added[0].Value = "changed second save"

	assert.Equal(t, []chathistory.Entry{reactionEntry(70, 10)}, original)
	assert.Equal(t, []chathistory.Entry{reactionEntry(70, 10), reactionEntry(71, 11)}, firstSave)
	assert.Equal(t, []chathistory.Entry{
		reactionEntry(70, 10),
		reactionEntry(71, 11),
		{
			ID:   20,
			Text: "next message",
		},
	}, buf.Get(1))
}

// spec: HIST-031
// spec: HIST-030
func TestBuffer_AddReaction_ConcurrentDuplicateSavesOnce(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	buf := chathistory.NewBuffer(100, chathistory.WithStore(store))

	var wg sync.WaitGroup

	for range 50 {
		wg.Go(func() {
			if err := buf.AddReaction(context.Background(), 1, reactionEntry(70, 10)); err != nil {
				t.Errorf("add reaction: %v", err)
			}

			snapshot := buf.Get(1)
			if len(snapshot) > 0 {
				snapshot[0].Reaction.Added[0].Value = "snapshot mutation"
			}
		})
	}

	wg.Wait()

	assert.Equal(t, []chathistory.Entry{reactionEntry(70, 10)}, buf.Get(1))
	assert.Equal(t, 1, store.saveCount(1))
	require.NoError(t, buf.AddReaction(context.Background(), 1, reactionEntry(71, 10)))
	assert.Equal(t, []chathistory.Entry{reactionEntry(70, 10), reactionEntry(71, 10)}, buf.Get(1))
	assert.Equal(t, 2, store.saveCount(1))
}

// spec: HIST-031
func TestBuffer_AddReaction_DedupeExpiresWithWindow(t *testing.T) {
	t.Parallel()

	buf := chathistory.NewBuffer(1)

	require.NoError(t, buf.AddReaction(context.Background(), 1, reactionEntry(70, 10)))

	buf.Add(1, chathistory.Entry{
		ID:   11,
		Text: "evicts reaction",
	})

	require.NoError(t, buf.AddReaction(context.Background(), 1, reactionEntry(70, 10)))
	assert.Equal(t, []chathistory.Entry{reactionEntry(70, 10)}, buf.Get(1))
}

// spec: HIST-032
func TestBuffer_AddReaction_LoadFailureRetriesWithoutSave(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.preload[1] = []chathistory.Entry{
		{
			ID:       10,
			SenderID: 101,
			Author:   "alice",
			Text:     "persisted",
		},
	}

	loadErr := errors.New("temporary storage outage")
	store.setLoadErr(loadErr)
	buf := chathistory.NewBuffer(
		10,
		chathistory.WithStore(store),
		chathistory.WithLogger(logger.New()),
	)

	err := buf.AddReaction(context.Background(), 1, reactionEntry(70, 10))

	require.ErrorIs(t, err, loadErr)
	assert.Zero(t, store.saveCount(1), "a failed Load must not overwrite the persisted blob")
	assert.Empty(t, buf.Get(1), "a failed initial load must not append an event in memory")

	store.setLoadErr(nil)

	require.NoError(t, buf.AddReaction(context.Background(), 1, reactionEntry(70, 10)))
	require.NoError(t, buf.AddReaction(context.Background(), 1, reactionEntry(70, 10)))

	got := buf.Get(1)

	require.Len(t, got, 2)
	assert.Equal(
		t,
		chathistory.Entry{
			ID:       10,
			SenderID: 101,
			Author:   "alice",
			Text:     "persisted",
		},
		got[0],
	)
	assert.Equal(t, 70, got[1].Reaction.UpdateID)
	assert.True(t, got[1].Reaction.TargetKnown)
	assert.Equal(t, int64(101), got[1].Reaction.TargetSenderID)
	assert.Equal(t, 1, store.saveCount(1))
	assert.Equal(t, got, store.lastSaved(1))
	assert.Equal(t, 3, store.loadCount(1), "failed initial loads must remain retryable")
}

// spec: HIST-031
func TestBuffer_AddReaction_SaveFailureRetainsMemoryAndDedupe(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.saveErr = errors.New("disk full")
	buf := chathistory.NewBuffer(
		10,
		chathistory.WithStore(store),
		chathistory.WithLogger(logger.New()),
	)

	require.NoError(t, buf.AddReaction(context.Background(), 1, reactionEntry(70, 10)))
	require.NoError(t, buf.AddReaction(context.Background(), 1, reactionEntry(70, 10)))

	assert.Equal(t, []chathistory.Entry{reactionEntry(70, 10)}, buf.Get(1))
	assert.Equal(t, 1, store.saveCount(1))
}

func reactionEntry(updateID, messageID int) chathistory.Entry {
	return chathistory.Entry{
		Time: time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC),
		Reaction: &chathistory.Reaction{
			UpdateID:  updateID,
			MessageID: messageID,
			ActorID:   202,
			Actor:     "bob",
			Added: []chathistory.ReactionType{
				{
					Type:  "emoji",
					Value: "👍",
				},
			},
			Removed: []chathistory.ReactionType{
				{
					Type:  "custom_emoji",
					Value: "123",
				},
			},
		},
	}
}

type retainingReactionStore struct {
	loaded []chathistory.Entry
	saved  []chathistory.Entry
}

func (s *retainingReactionStore) Load(context.Context, int64) ([]chathistory.Entry, error) {
	return s.loaded, nil
}

func (s *retainingReactionStore) Save(
	_ context.Context,
	_ int64,
	entries []chathistory.Entry,
) error {
	s.saved = entries

	return nil
}
