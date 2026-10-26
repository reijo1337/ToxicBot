package db

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/reijo1337/ToxicBot/internal/domain/chat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// spec: SET-023
func TestChatSettingsStorage_TaggerEnabledPersists(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "bot.db")
	m := migrateFile(t, path)
	ctx := context.Background()
	off := false
	count := 7

	conn, err := sqlx.Open("sqlite3", path)
	require.NoError(t, err)
	s := NewChatSettingsStorage(NewConnGetter(conn))
	require.NoError(t, s.Upsert(ctx, 1, chat.ChatSettings{TaggerEnabled: &off}))
	require.NoError(t, s.Upsert(ctx, 1, chat.ChatSettings{ThresholdCount: &count}))
	require.NoError(t, conn.Close())

	conn, err = sqlx.Open("sqlite3", path)
	require.NoError(t, err)
	s = NewChatSettingsStorage(NewConnGetter(conn))
	got, err := s.Get(ctx, 1)
	require.NoError(t, err)
	require.NotNil(t, got.TaggerEnabled, "другое поле не затирает выключатель")
	assert.False(t, *got.TaggerEnabled)
	assert.Equal(t, 7, *got.ThresholdCount)

	require.NoError(t, s.Upsert(ctx, 2, chat.ChatSettings{ThresholdCount: &count}))
	other, err := s.Get(ctx, 2)
	require.NoError(t, err)
	assert.Nil(t, other.TaggerEnabled, "без переопределения — NULL")
	require.NoError(t, conn.Close())

	require.NoError(t, m.Migrate(5), "down 000006 удаляет колонку")
	conn, err = sqlx.Open("sqlite3", path)
	require.NoError(t, err)
	_, err = NewChatSettingsStorage(NewConnGetter(conn)).Get(ctx, 1)
	require.Error(t, err, "колонки tagger_enabled больше нет")
	require.NoError(t, conn.Close())
	require.NoError(t, m.Up())
}
