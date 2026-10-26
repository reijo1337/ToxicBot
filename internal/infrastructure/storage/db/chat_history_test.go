package db

import (
	"bytes"
	"context"
	"encoding/gob"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupChatHistoryDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	_, err = db.ExecContext(context.Background(), `create table chat_history (
		chat_id integer primary key,
		data    blob not null
	) without rowid`)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// spec: HIST-015
func TestChatHistoryStorage_Load_NoRows(t *testing.T) {
	t.Parallel()

	s := NewChatHistoryStorage(NewConnGetter(setupChatHistoryDB(t)))

	entries, err := s.Load(context.Background(), 42)

	require.NoError(t, err)
	assert.Nil(t, entries)
}

// spec: HIST-016
func TestChatHistoryStorage_SaveLoad_RoundTrip(t *testing.T) {
	t.Parallel()

	s := NewChatHistoryStorage(NewConnGetter(setupChatHistoryDB(t)))
	ctx := context.Background()

	in := []chathistory.Entry{
		{
			ID:     1,
			Time:   time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC),
			Author: "@alice",
			Text:   "привет",
		},
		{
			ID:           2,
			Time:         time.Date(2026, 4, 30, 10, 1, 0, 0, time.UTC),
			Author:       "бот",
			Text:         "<photo>desc</photo>",
			ReplyToID:    1,
			FromBot:      true,
			PreFormatted: true,
		},
	}

	require.NoError(t, s.Save(ctx, 42, in))

	out, err := s.Load(ctx, 42)
	require.NoError(t, err)
	assert.Equal(t, in, out)
}

// spec: HIST-017
func TestChatHistoryStorage_Save_Replaces(t *testing.T) {
	t.Parallel()

	s := NewChatHistoryStorage(NewConnGetter(setupChatHistoryDB(t)))
	ctx := context.Background()

	require.NoError(t, s.Save(ctx, 42, []chathistory.Entry{{ID: 1, Text: "first"}}))
	require.NoError(t, s.Save(ctx, 42, []chathistory.Entry{{ID: 2, Text: "second"}}))

	out, err := s.Load(ctx, 42)
	require.NoError(t, err)
	assert.Equal(t, []chathistory.Entry{{ID: 2, Text: "second"}}, out)
}

// spec: HIST-018
func TestChatHistoryStorage_IsolatedByChat(t *testing.T) {
	t.Parallel()

	s := NewChatHistoryStorage(NewConnGetter(setupChatHistoryDB(t)))
	ctx := context.Background()

	require.NoError(t, s.Save(ctx, 1, []chathistory.Entry{{ID: 10, Text: "chat1"}}))
	require.NoError(t, s.Save(ctx, 2, []chathistory.Entry{{ID: 20, Text: "chat2"}}))

	out1, err := s.Load(ctx, 1)
	require.NoError(t, err)
	out2, err := s.Load(ctx, 2)
	require.NoError(t, err)

	require.Len(t, out1, 1)
	require.Len(t, out2, 1)
	assert.Equal(t, "chat1", out1[0].Text)
	assert.Equal(t, "chat2", out2[0].Text)
}

// wireHistoryEntry models the persisted schema before link context was added.
type wireHistoryEntry struct {
	ID           int
	SenderID     int64
	Time         time.Time
	Author       string
	Text         string
	ReplyToID    int
	FromBot      bool
	PreFormatted bool
}

// spec: HIST-019
func TestChatHistoryStorage_SenderSurvivesRestart(t *testing.T) {
	t.Parallel()
	conn := setupChatHistoryDB(t)
	ctx := context.Background()
	entries := []wireHistoryEntry{
		{ID: 1, SenderID: 101, Author: "Alex", Text: "first"},
		{ID: 2, SenderID: 202, Author: "Alex", Text: "second", ReplyToID: 1},
	}
	var blob bytes.Buffer
	require.NoError(t, gob.NewEncoder(&blob).Encode(entries))
	_, err := conn.ExecContext(ctx, "insert into chat_history values (?, ?)", 42, blob.Bytes())
	require.NoError(t, err)
	first := NewChatHistoryStorage(NewConnGetter(conn))
	buffer := chathistory.NewBuffer(100, chathistory.WithStore(first))
	loaded := buffer.Get(42)
	require.Len(t, loaded, 2)
	require.NoError(t, first.Save(ctx, 42, loaded))
	restarted := chathistory.NewBuffer(
		100,
		chathistory.WithStore(NewChatHistoryStorage(NewConnGetter(conn))),
	)
	require.Equal(t, loaded, restarted.Get(42))
	var persisted []byte
	require.NoError(
		t,
		conn.GetContext(ctx, &persisted, "select data from chat_history where chat_id = ?", 42),
	)
	var got []wireHistoryEntry
	require.NoError(t, gob.NewDecoder(bytes.NewReader(persisted)).Decode(&got))
	assert.Equal(t, entries, got)
}

// legacyHistoryEntry is the exact persisted schema before SenderID was added.
type legacyHistoryEntry struct {
	ID           int
	Time         time.Time
	Author       string
	Text         string
	ReplyToID    int
	FromBot      bool
	PreFormatted bool
}

// spec: HIST-020
func TestChatHistoryStorage_LoadLegacyPreservesUnknownAuthorship(t *testing.T) {
	t.Parallel()
	conn := setupChatHistoryDB(t)
	ctx := context.Background()
	stamp := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	legacy := []legacyHistoryEntry{
		{ID: 1, Time: stamp, Author: "Alex", Text: "legacy"},
		{
			ID:           2,
			Time:         stamp.Add(time.Second),
			Author:       "@bot",
			Text:         "<photo>old</photo>",
			ReplyToID:    1,
			FromBot:      true,
			PreFormatted: true,
		},
	}
	var blob bytes.Buffer
	require.NoError(t, gob.NewEncoder(&blob).Encode(legacy))
	_, err := conn.ExecContext(ctx, "insert into chat_history values (?, ?)", 42, blob.Bytes())
	require.NoError(t, err)
	storage := NewChatHistoryStorage(NewConnGetter(conn))
	got, err := storage.Load(ctx, 42)
	require.NoError(t, err)
	assert.Equal(t, []chathistory.Entry{
		{ID: 1, Time: stamp, Author: "Alex", Text: "legacy"},
		{
			ID:           2,
			Time:         stamp.Add(time.Second),
			Author:       "@bot",
			Text:         "<photo>old</photo>",
			ReplyToID:    1,
			FromBot:      true,
			PreFormatted: true,
		},
	}, got)
}

// spec: HIST-021
func TestChatHistoryStorage_BufferRestartPreservesMixedHistory(t *testing.T) {
	t.Parallel()
	conn := setupChatHistoryDB(t)
	ctx := context.Background()
	stamp := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	var blob bytes.Buffer
	require.NoError(t, gob.NewEncoder(&blob).Encode([]legacyHistoryEntry{
		{ID: 1, Time: stamp, Author: "Alex", Text: "legacy", ReplyToID: 50},
	}))
	_, err := conn.ExecContext(ctx, "insert into chat_history values (?, ?)", 42, blob.Bytes())
	require.NoError(t, err)
	buffer := chathistory.NewBuffer(
		100,
		chathistory.WithStore(NewChatHistoryStorage(NewConnGetter(conn))),
	)
	buffer.AddAll(
		42,
		chathistory.Entry{
			ID:        2,
			SenderID:  101,
			Time:      stamp.Add(time.Second),
			Author:    "Alex",
			Text:      "first",
			ReplyToID: 1,
		},
		chathistory.Entry{
			ID:        3,
			SenderID:  202,
			Time:      stamp.Add(2 * time.Second),
			Author:    "Alex",
			Text:      "second",
			ReplyToID: 2,
		},
	)
	restarted := chathistory.NewBuffer(
		100,
		chathistory.WithStore(NewChatHistoryStorage(NewConnGetter(conn))),
	)
	assert.Equal(t, []chathistory.Entry{
		{ID: 1, Time: stamp, Author: "Alex", Text: "legacy", ReplyToID: 50},
		{
			ID:        2,
			SenderID:  101,
			Time:      stamp.Add(time.Second),
			Author:    "Alex",
			Text:      "first",
			ReplyToID: 1,
		},
		{
			ID:        3,
			SenderID:  202,
			Time:      stamp.Add(2 * time.Second),
			Author:    "Alex",
			Text:      "second",
			ReplyToID: 2,
		},
	}, restarted.Get(42))
	// Loading a legacy chat after a populated new chat must not reuse its SenderID.
	require.NoError(t, NewChatHistoryStorage(NewConnGetter(conn)).Save(ctx, 43,
		[]chathistory.Entry{{ID: 10, SenderID: 999, Author: "Alex"}}))
	storage := NewChatHistoryStorage(NewConnGetter(conn))
	_, err = storage.Load(ctx, 43)
	require.NoError(t, err)
	got, err := storage.Load(ctx, 42)
	require.NoError(t, err)
	assert.Zero(t, got[0].SenderID)
}

// spec: HIST-020
// spec: HIST-021
func TestChatHistoryStorage_NewGobReadableAfterRollback(t *testing.T) {
	t.Parallel()
	conn := setupChatHistoryDB(t)
	ctx := context.Background()
	storage := NewChatHistoryStorage(NewConnGetter(conn))
	stamp := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	require.NoError(t, storage.Save(ctx, 42, []chathistory.Entry{
		{ID: 1, SenderID: 101, Time: stamp, Author: "Alex", Text: "first"},
		{
			ID:           2,
			Time:         stamp.Add(time.Second),
			Author:       "@bot",
			Text:         "<photo>data</photo>",
			ReplyToID:    1,
			FromBot:      true,
			PreFormatted: true,
		},
	}))
	var blob []byte
	require.NoError(
		t,
		conn.GetContext(ctx, &blob, "select data from chat_history where chat_id = ?", 42),
	)
	var legacy []legacyHistoryEntry
	require.NoError(t, gob.NewDecoder(bytes.NewReader(blob)).Decode(&legacy))
	assert.Equal(t, []legacyHistoryEntry{
		{ID: 1, Time: stamp, Author: "Alex", Text: "first"},
		{
			ID:           2,
			Time:         stamp.Add(time.Second),
			Author:       "@bot",
			Text:         "<photo>data</photo>",
			ReplyToID:    1,
			FromBot:      true,
			PreFormatted: true,
		},
	}, legacy)
	// A downgraded writer loses only the unknown new field. Upgrading again is safe.
	var rewritten bytes.Buffer
	require.NoError(t, gob.NewEncoder(&rewritten).Encode(legacy))
	_, err := conn.ExecContext(
		ctx,
		"update chat_history set data = ? where chat_id = ?",
		rewritten.Bytes(),
		42,
	)
	require.NoError(t, err)
	restored, err := storage.Load(ctx, 42)
	require.NoError(t, err)
	assert.Equal(t, []chathistory.Entry{
		{ID: 1, Time: stamp, Author: "Alex", Text: "first"},
		{
			ID:           2,
			Time:         stamp.Add(time.Second),
			Author:       "@bot",
			Text:         "<photo>data</photo>",
			ReplyToID:    1,
			FromBot:      true,
			PreFormatted: true,
		},
	}, restored)
}

// spec: HIST-023
// spec: HIST-025
func TestChatHistoryStorage_LinkSurvivesBufferRestart(t *testing.T) {
	t.Parallel()
	conn := setupChatHistoryDB(t)
	first := chathistory.NewBuffer(
		100,
		chathistory.WithStore(NewChatHistoryStorage(NewConnGetter(conn))),
	)
	entry := chathistory.Entry{
		ID: 10, SenderID: 101, Time: time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC),
		Author: "@alice", Text: "почитай тут", ReplyToID: 9,
		Link: chathistory.Link{
			URL: "https://example.com/article", Anchor: "почитай тут", Title: "Заголовок",
			Description: "Описание материала", PreviewDescription: "Пересказ превью",
		},
	}
	want := entry
	first.Add(42, entry)
	entry.Link.Anchor = "изменение входа"

	restarted := chathistory.NewBuffer(
		100,
		chathistory.WithStore(NewChatHistoryStorage(NewConnGetter(conn))),
	)
	got := restarted.Get(42)
	assert.Equal(t, []chathistory.Entry{want}, got)
	got[0].Link.PreviewDescription = "изменение снимка"
	assert.Equal(t, []chathistory.Entry{want}, restarted.Get(42))

	storage := NewChatHistoryStorage(NewConnGetter(conn))
	persisted, err := storage.Load(context.Background(), 42)
	require.NoError(t, err)
	assert.Equal(t, []chathistory.Entry{want}, persisted)
}

// spec: HIST-024
func TestChatHistoryStorage_PreLinkGobKeepsAuthorshipAndNoEnrichment(t *testing.T) {
	t.Parallel()
	conn := setupChatHistoryDB(t)
	ctx := context.Background()
	stamp := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	legacy := []wireHistoryEntry{
		{
			ID:        1,
			SenderID:  101,
			Time:      stamp,
			Author:    "@alice",
			Text:      "https://example.com/old",
			ReplyToID: 9,
		},
		{
			ID: 2, Time: stamp.Add(time.Second), Author: "@bot", Text: "old reply",
			ReplyToID: 1, FromBot: true, PreFormatted: true,
		},
	}
	var blob bytes.Buffer
	require.NoError(t, gob.NewEncoder(&blob).Encode(legacy))
	_, err := conn.ExecContext(ctx, "insert into chat_history values (?, ?)", 42, blob.Bytes())
	require.NoError(t, err)
	buffer := chathistory.NewBuffer(
		100,
		chathistory.WithStore(NewChatHistoryStorage(NewConnGetter(conn))),
	)
	want := []chathistory.Entry{
		{
			ID:        1,
			SenderID:  101,
			Time:      stamp,
			Author:    "@alice",
			Text:      "https://example.com/old",
			ReplyToID: 9,
		},
		{
			ID: 2, Time: stamp.Add(time.Second), Author: "@bot", Text: "old reply",
			ReplyToID: 1, FromBot: true, PreFormatted: true,
		},
	}
	assert.Equal(t, want, buffer.Get(42))
	assert.Equal(t, want, buffer.Get(42))
}

// spec: HIST-024
func TestChatHistoryStorage_LinkGobReadableByPreviousVersion(t *testing.T) {
	t.Parallel()
	conn := setupChatHistoryDB(t)
	ctx := context.Background()
	storage := NewChatHistoryStorage(NewConnGetter(conn))
	stamp := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	entry := chathistory.Entry{
		ID: 1, SenderID: 101, Time: stamp, Author: "@alice", Text: "почитай тут", ReplyToID: 9,
		Link: chathistory.Link{
			URL: "https://example.com", Anchor: "почитай тут", Title: "Заголовок",
			Description: "Описание", PreviewDescription: "Превью",
		},
	}
	require.NoError(t, storage.Save(ctx, 42, []chathistory.Entry{entry}))
	var blob []byte
	require.NoError(
		t,
		conn.GetContext(ctx, &blob, "select data from chat_history where chat_id = ?", 42),
	)
	var previous []wireHistoryEntry
	require.NoError(t, gob.NewDecoder(bytes.NewReader(blob)).Decode(&previous))
	assert.Equal(t, []wireHistoryEntry{{
		ID: 1, SenderID: 101, Time: stamp, Author: "@alice", Text: "почитай тут", ReplyToID: 9,
	}}, previous)

	var rewritten bytes.Buffer
	require.NoError(t, gob.NewEncoder(&rewritten).Encode(previous))
	_, err := conn.ExecContext(
		ctx,
		"update chat_history set data = ? where chat_id = ?",
		rewritten.Bytes(),
		42,
	)
	require.NoError(t, err)
	upgraded, err := storage.Load(ctx, 42)
	require.NoError(t, err)
	entry.Link = chathistory.Link{}
	assert.Equal(t, []chathistory.Entry{entry}, upgraded)
}
