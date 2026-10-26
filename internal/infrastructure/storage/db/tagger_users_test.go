package db

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jmoiron/sqlx"
	"github.com/reijo1337/ToxicBot/internal/domain/chat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const taggerTestChat = int64(-1001)

// migrateFile применяет настоящие миграции репозитория к файловой SQLite.
func migrateFile(t *testing.T, path string) *migrate.Migrate {
	t.Helper()
	dir, err := filepath.Abs("../../../../db/migrations")
	require.NoError(t, err)
	m, err := migrate.New("file://"+dir, "sqlite3://"+path)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = m.Close() })
	require.NoError(t, m.Up())
	return m
}

func openTaggerStorage(t *testing.T, path string) (*TaggerUsersStorage, func()) {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", path)
	require.NoError(t, err)
	return NewTaggerUsersStorage(NewConnGetter(conn)), func() { require.NoError(t, conn.Close()) }
}

func newTaggerDBFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bot.db")
	migrateFile(t, path)
	return path
}

func TestTaggerUsersMigration_UpDown(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "bot.db")
	m := migrateFile(t, path)

	s, closeDB := openTaggerStorage(t, path)
	ctx := context.Background()
	require.NoError(t, s.Upsert(ctx, chat.TaggerUser{ChatID: 1, UserID: 2, Author: "@a"}))
	closeDB()

	require.NoError(t, m.Migrate(5), "down до версии перед tagger_users")

	s, closeDB = openTaggerStorage(t, path)
	_, err := s.LoadAll(ctx)
	require.Error(t, err, "после down таблицы нет")
	closeDB()

	require.NoError(t, m.Up())

	s, closeDB = openTaggerStorage(t, path)
	defer closeDB()
	users, err := s.LoadAll(ctx)
	require.NoError(t, err)
	assert.Empty(t, users, "down удаляет накопленный список")
}

func TestTaggerUsersStorage_LoadAll_Empty(t *testing.T) {
	t.Parallel()

	s, closeDB := openTaggerStorage(t, newTaggerDBFile(t))
	defer closeDB()

	users, err := s.LoadAll(context.Background())

	require.NoError(t, err)
	assert.Empty(t, users)
}

// spec: TAG-014
func TestTaggerUsersStorage_SurvivesReopenWithoutDuplicates(t *testing.T) {
	t.Parallel()

	path := newTaggerDBFile(t)
	ctx := context.Background()

	s, closeDB := openTaggerStorage(t, path)
	require.NoError(
		t,
		s.Upsert(ctx, chat.TaggerUser{ChatID: taggerTestChat, UserID: 10, Author: "@old"}),
	)
	require.NoError(
		t,
		s.Upsert(ctx, chat.TaggerUser{ChatID: taggerTestChat, UserID: 20, Author: "Боб"}),
	)
	require.NoError(
		t,
		s.Upsert(ctx, chat.TaggerUser{ChatID: taggerTestChat, UserID: 10, Author: "@new"}),
	)
	closeDB()

	s, closeDB = openTaggerStorage(t, path)
	defer closeDB()
	users, err := s.LoadAll(ctx)

	require.NoError(t, err)
	assert.Equal(t, []chat.TaggerUser{
		{ChatID: taggerTestChat, UserID: 10, Author: "@new"},
		{ChatID: taggerTestChat, UserID: 20, Author: "Боб"},
	}, users)
}

// spec: TAG-016
// spec: TAG-017
func TestTaggerUsersStorage_DeleteIsolatedByChat(t *testing.T) {
	t.Parallel()

	path := newTaggerDBFile(t)
	ctx := context.Background()

	s, closeDB := openTaggerStorage(t, path)
	require.NoError(t, s.Upsert(ctx, chat.TaggerUser{ChatID: 1, UserID: 10, Author: "@a"}))
	require.NoError(t, s.Upsert(ctx, chat.TaggerUser{ChatID: 2, UserID: 10, Author: "@a-в-другом"}))
	require.NoError(t, s.Upsert(ctx, chat.TaggerUser{ChatID: 1, UserID: 20, Author: "@b"}))
	require.NoError(t, s.Upsert(ctx, chat.TaggerUser{ChatID: 1, UserID: 10, Author: "@a2"}))
	require.NoError(t, s.Delete(ctx, 1, 20))
	closeDB()

	s, closeDB = openTaggerStorage(t, path)
	defer closeDB()
	users, err := s.LoadAll(ctx)

	require.NoError(t, err)
	assert.Equal(t, []chat.TaggerUser{
		{ChatID: 1, UserID: 10, Author: "@a2"},
		{ChatID: 2, UserID: 10, Author: "@a-в-другом"},
	}, users)
}

// spec: TAG-018
func TestTaggerUsersStorage_DeleteChat(t *testing.T) {
	t.Parallel()

	path := newTaggerDBFile(t)
	ctx := context.Background()

	s, closeDB := openTaggerStorage(t, path)
	require.NoError(t, s.Upsert(ctx, chat.TaggerUser{ChatID: 1, UserID: 10, Author: "@a"}))
	require.NoError(t, s.Upsert(ctx, chat.TaggerUser{ChatID: 1, UserID: 20, Author: "@b"}))
	require.NoError(t, s.Upsert(ctx, chat.TaggerUser{ChatID: 2, UserID: 10, Author: "@a"}))
	require.NoError(t, s.DeleteChat(ctx, 1))
	closeDB()

	s, closeDB = openTaggerStorage(t, path)
	defer closeDB()
	users, err := s.LoadAll(ctx)

	require.NoError(t, err)
	assert.Equal(t, []chat.TaggerUser{{ChatID: 2, UserID: 10, Author: "@a"}}, users)
}

// spec: TAG-021
func TestTaggerUsersStorage_ReturnsWriteErrors(t *testing.T) {
	t.Parallel()

	conn, err := sqlx.Open("sqlite3", newTaggerDBFile(t))
	require.NoError(t, err)
	s := NewTaggerUsersStorage(NewConnGetter(conn))
	require.NoError(t, conn.Close())
	ctx := context.Background()

	_, loadErr := s.LoadAll(ctx)

	require.Error(t, s.Upsert(ctx, chat.TaggerUser{ChatID: 1, UserID: 2, Author: "@a"}))
	require.Error(t, s.Delete(ctx, 1, 2))
	require.Error(t, s.DeleteChat(ctx, 1))
	require.Error(t, loadErr)
}
