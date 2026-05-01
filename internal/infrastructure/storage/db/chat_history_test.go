package db

import (
	"context"
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

func TestChatHistoryStorage_Load_NoRows(t *testing.T) {
	t.Parallel()

	s := NewChatHistoryStorage(NewConnGetter(setupChatHistoryDB(t)))

	entries, err := s.Load(context.Background(), 42)

	require.NoError(t, err)
	assert.Nil(t, entries)
}

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
