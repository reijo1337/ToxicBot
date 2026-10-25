package tagger

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/reijo1337/ToxicBot/internal/domain/chat"
	"github.com/reijo1337/ToxicBot/internal/infrastructure/storage/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/telebot.v3"
)

var errDisk = errors.New("disk I/O error")

// applySchema создаёт в новом файле SQLite схему из настоящих миграций.
func applySchema(t *testing.T, path string) {
	t.Helper()
	dir, err := filepath.Abs("../../../db/migrations")
	require.NoError(t, err)
	m, err := migrate.New("file://"+dir, "sqlite3://"+path)
	require.NoError(t, err)
	require.NoError(t, m.Up())
	srcErr, dbErr := m.Close()
	require.NoError(t, srcErr)
	require.NoError(t, dbErr)
}

func openSQLite(t *testing.T, path string) (*db.TaggerUsersStorage, func()) {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", path)
	require.NoError(t, err)
	closeDB := func() { require.NoError(t, conn.Close()) }
	return db.NewTaggerUsersStorage(db.NewConnGetter(conn)), closeDB
}

func TestMembershipOf(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		member *telebot.ChatMember
		want   membership
	}{
		"creator": {&telebot.ChatMember{Role: telebot.Creator}, membershipPresent},
		"administrator": {
			&telebot.ChatMember{Role: telebot.Administrator},
			membershipPresent,
		},
		"member": {&telebot.ChatMember{Role: telebot.Member}, membershipPresent},
		"restricted, is_member": {
			&telebot.ChatMember{Role: telebot.Restricted, Member: true},
			membershipPresent,
		},
		"restricted, вышел":  {&telebot.ChatMember{Role: telebot.Restricted}, membershipGone},
		"left":               {&telebot.ChatMember{Role: telebot.Left}, membershipGone},
		"kicked":             {&telebot.ChatMember{Role: telebot.Kicked}, membershipGone},
		"неизвестный статус": {&telebot.ChatMember{Role: "banana"}, membershipUnknown},
		"пустой ответ":       {nil, membershipUnknown},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, membershipOf(tc.member))
		})
	}
}

// spec: TAG-007
func TestHandle_MemberBecomesPersistedCandidate(t *testing.T) {
	t.Parallel()

	st := newMemStorage()
	s := newStand(t, st)

	require.NoError(t, s.h.Handle(s.message(groupA, user(1, "alice"))))
	require.NoError(t, s.h.Handle(s.message(groupA, user(1, "alice"))))

	assert.Equal(t, map[int64]string{1: "@alice"}, s.candidates(groupA))
	assert.Equal(t, []chat.TaggerUser{{ChatID: groupA, UserID: 1, Author: "@alice"}}, st.users())
	assert.Len(t, s.jobsOf(groupA), 1)
}

// spec: TAG-008
func TestHandle_LeftOrKickedIsNotCandidate(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"left":                  `{"status":"left","user":{"id":2}}`,
		"kicked":                `{"status":"kicked","user":{"id":2}}`,
		"restricted, не в чате": `{"status":"restricted","is_member":false,"user":{"id":2}}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			st := newMemStorage(chat.TaggerUser{ChatID: groupA, UserID: 2, Author: "@old"})
			s := newStand(t, st)
			s.api.setMember(2, raw)
			s.api.setMember(3, raw)

			require.NoError(t, s.h.Handle(s.message(groupA, user(2, "old"))))
			require.NoError(t, s.h.Handle(s.message(groupA, user(3, "ghost"))))

			assert.Empty(t, s.candidates(groupA), "подтверждённое отсутствие убирает и известного")
			assert.Empty(t, st.users())
			assert.Empty(t, s.jobsOf(groupA))
		})
	}
}

// spec: TAG-023
func TestHandle_BotsAreNeverCandidates(t *testing.T) {
	t.Parallel()

	s := newStand(t, newMemStorage())
	bot := &telebot.User{ID: 5, Username: "other_bot", IsBot: true}

	require.NoError(t, s.h.Handle(s.message(groupA, bot)))
	require.NoError(t, s.h.OnJoin().Handle(s.joined(groupA, bot, bot)))

	assert.Empty(t, s.candidates(groupA))
	assert.Empty(t, s.api.memberChecks(), "бот отсекается до запроса к Telegram")
}

// spec: TAG-023
func TestOnJoin_JoinedIsCandidateWithoutMessage(t *testing.T) {
	t.Parallel()

	st := newMemStorage()
	s := newStand(t, st)
	inviter := user(7, "inviter")
	joined := user(8, "newbie")

	require.NoError(t, s.h.OnJoin().Handle(s.joined(groupA, inviter, joined)))

	assert.Equal(t, map[int64]string{7: "@inviter", 8: "@newbie"}, s.candidates(groupA))
	assert.Equal(t, []string{"7"}, s.api.memberChecks(), "проверяется только пригласивший")
	assert.Len(t, st.users(), 2)
	assert.Len(t, s.jobsOf(groupA), 1)

	restarted := newStand(t, st)
	assert.Equal(t, s.candidates(groupA), restarted.candidates(groupA))
}

// spec: TAG-017
// spec: TAG-016
func TestOnLeft_RemovesOnlyThatChatMember(t *testing.T) {
	t.Parallel()

	st := newMemStorage(
		chat.TaggerUser{ChatID: groupA, UserID: 1, Author: "@alice"},
		chat.TaggerUser{ChatID: groupA, UserID: 2, Author: "@bob"},
		chat.TaggerUser{ChatID: groupB, UserID: 1, Author: "@alice"},
	)
	s := newStand(t, st)
	admin := user(3, "admin")

	require.NoError(t, s.h.OnLeft().Handle(s.left(admin, user(1, "alice"))))

	assert.Equal(t, map[int64]string{2: "@bob", 3: "@admin"}, s.candidates(groupA))
	assert.Equal(t, []string{"3"}, s.api.memberChecks(), "удаляется UserLeft, а не инициатор")

	restarted := newStand(t, st)
	assert.Equal(t, map[int64]string{2: "@bob", 3: "@admin"}, restarted.candidates(groupA))
	assert.Equal(t, map[int64]string{1: "@alice"}, restarted.candidates(groupB))
}

// spec: TAG-018
func TestOnLeft_BotRemovedForgetsWholeChat(t *testing.T) {
	t.Parallel()

	st := newMemStorage(
		chat.TaggerUser{ChatID: groupA, UserID: 1, Author: "@alice"},
		chat.TaggerUser{ChatID: groupA, UserID: 2, Author: "@bob"},
		chat.TaggerUser{ChatID: groupB, UserID: 1, Author: "@alice"},
	)
	s := newStand(t, st)
	require.Len(t, s.jobsOf(groupA), 1)

	me := &telebot.User{ID: botID, IsBot: true}
	require.NoError(t, s.h.OnLeft().Handle(s.left(user(4, "kicker"), me)))

	assert.Empty(t, s.candidates(groupA))
	assert.Empty(t, s.jobsOf(groupA), "пинги в чат прекращаются")
	assert.Empty(t, s.api.memberChecks(), "инициатор удаления не возвращается в список")

	s.h.tick(context.Background(), due())
	for _, m := range s.api.sentMessages() {
		assert.NotEqual(t, strconv.FormatInt(groupA, 10), m.chatID)
	}

	restarted := newStand(t, st)
	assert.Empty(t, restarted.candidates(groupA))
	assert.Empty(t, restarted.jobsOf(groupA))
	assert.Len(t, restarted.jobsOf(groupB), 1)
}

// spec: TAG-014
// spec: TAG-015
// spec: TAG-012
func TestRestart_ResumesPingsFromSQLiteWithoutNewMessages(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "bot.db")
	applySchema(t, path)
	storage, closeDB := openSQLite(t, path)
	s := newStand(t, storage)

	require.NoError(t, s.h.Handle(s.message(groupA, user(1, "alice"))))
	require.NoError(t, s.h.OnJoin().Handle(s.joined(groupA, user(1, "alice"), user(2, "bob"))))
	require.NoError(t, s.h.Handle(s.message(groupB, user(1, "alice"))))
	require.NoError(t, s.h.Handle(s.message(groupA, user(1, "alice_renamed"))))
	closeDB()

	storage, closeDB = openSQLite(t, path)
	defer closeDB()
	before := time.Now()
	restarted := newStandWithAPI(t, storage, &fakeTelegram{})

	assert.Equal(t, map[int64]string{1: "@alice_renamed", 2: "@bob"}, restarted.candidates(groupA))
	assert.Equal(t, map[int64]string{1: "@alice"}, restarted.candidates(groupB))
	for _, chatID := range []int64{groupA, groupB} {
		jobs := restarted.jobsOf(chatID)
		require.Len(t, jobs, 1, "один пинг на чат")
		assert.False(t, jobs[0].tagAt.Before(before.Add(tagFrom)), "новый случайный интервал")
	}

	restarted.h.tick(context.Background(), due())
	restarted.h.tick(context.Background(), due())

	sent := restarted.api.sentMessages()
	require.Len(t, sent, 2)
	assert.ElementsMatch(
		t,
		[]string{strconv.FormatInt(groupA, 10), strconv.FormatInt(groupB, 10)},
		[]string{sent[0].chatID, sent[1].chatID},
	)
}

func TestNew_EmptyRegistrySchedulesNothing(t *testing.T) {
	t.Parallel()

	s := newStand(t, newMemStorage())

	assert.Empty(t, s.h.queue)
	s.h.tick(context.Background(), due())
	assert.Empty(t, s.api.memberChecks())
}

// spec: TAG-021
func TestHandle_WriteFailureKeepsCandidatesAndJobs(t *testing.T) {
	t.Parallel()

	st := newMemStorage(chat.TaggerUser{ChatID: groupA, UserID: 1, Author: "@alice"})
	s := newStand(t, st)
	before := s.candidates(groupA)

	st.fail(&st.upsertErr, errDisk)
	require.ErrorIs(t, s.h.Handle(s.message(groupA, user(2, "bob"))), errDisk)
	require.ErrorIs(t, s.h.Handle(s.message(groupA, user(1, "alice_new"))), errDisk)
	require.ErrorIs(t, s.h.OnJoin().Handle(s.joined(groupB, nil, user(3, "carol"))), errDisk)

	st.fail(&st.deleteErr, errDisk)
	require.ErrorIs(t, s.h.OnLeft().Handle(s.left(nil, user(1, "alice"))), errDisk)

	st.fail(&st.deleteChatErr, errDisk)
	me := &telebot.User{ID: botID, IsBot: true}
	require.ErrorIs(t, s.h.OnLeft().Handle(s.left(nil, me)), errDisk)

	assert.Equal(t, before, s.candidates(groupA))
	assert.Empty(t, s.candidates(groupB))
	assert.Empty(t, s.jobsOf(groupB))
	assert.Len(t, s.jobsOf(groupA), 1)
}

// spec: TAG-021
// spec: TAG-019
func TestTick_ConfirmedLeftIsNotPingedEvenIfDeleteFails(t *testing.T) {
	t.Parallel()

	st := newMemStorage(chat.TaggerUser{ChatID: groupA, UserID: 1, Author: "@alice"})
	s := newStand(t, st)
	s.api.setMember(1, leftRaw)
	st.fail(&st.deleteErr, errDisk)

	s.h.tick(context.Background(), due())

	assert.Empty(t, s.api.sentMessages())
	assert.Contains(t, s.log.messages(), "can't forget left tagger candidate")
	assert.Equal(t, map[int64]string{1: "@alice"}, s.candidates(groupA), "память не опережает БД")
	assert.Len(t, s.jobsOf(groupA), 1)
}

// spec: TAG-019
func TestTick_LeftDuringDowntimeIsRemovedBeforePing(t *testing.T) {
	t.Parallel()

	st := newMemStorage(
		chat.TaggerUser{ChatID: groupA, UserID: 1, Author: "@gone"},
		chat.TaggerUser{ChatID: groupA, UserID: 2, Author: "@stay"},
	)
	s := newStand(t, st)
	s.api.setMember(1, `{"status":"kicked","user":{"id":1}}`)
	now := due()

	s.h.tick(context.Background(), now)

	assert.Empty(t, s.api.sentMessages())
	assert.Equal(t, map[int64]string{2: "@stay"}, s.candidates(groupA))
	assert.Equal(t, []chat.TaggerUser{{ChatID: groupA, UserID: 2, Author: "@stay"}}, st.users())
	jobs := s.jobsOf(groupA)
	require.Len(t, jobs, 1)
	assert.True(t, jobs[0].tagAt.After(time.Now().Add(tagFrom-time.Minute)), "обычный интервал")
}

// spec: TAG-020
func TestTick_UnverifiableMembershipSkipsPingAndKeepsCandidate(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"ошибка API":         `{"ok":false,"error_code":400,"description":"Bad Request: member not found"}`,
		"пустой ответ":       `{"ok":true,"result":null}`,
		"неизвестный статус": `{"ok":true,"result":{"status":"banana","user":{"id":1}}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			st := newMemStorage(chat.TaggerUser{ChatID: groupA, UserID: 1, Author: "@alice"})
			s := newStand(t, st)
			s.api.setRaw(1, body)

			s.h.tick(context.Background(), due())

			assert.Empty(t, s.api.sentMessages())
			assert.Equal(t, map[int64]string{1: "@alice"}, s.candidates(groupA))
			assert.Len(t, st.users(), 1)
			jobs := s.jobsOf(groupA)
			require.Len(t, jobs, 1)
			assert.True(t, jobs[0].tagAt.After(time.Now().Add(tagFrom-time.Minute)))
		})
	}
}

// spec: TAG-001
func TestTick_DuePingIsSentAndNextPlanned(t *testing.T) {
	t.Parallel()

	s := newStand(t, newMemStorage(chat.TaggerUser{ChatID: groupA, UserID: 1, Author: "@alice"}))
	first := s.jobsOf(groupA)[0]

	s.h.tick(context.Background(), due())

	sent := s.api.sentMessages()
	require.Len(t, sent, 1)
	assert.Equal(t, "[Валера](tg://user?id=1), чего молчишь", sent[0].text)
	jobs := s.jobsOf(groupA)
	require.Len(t, jobs, 1)
	assert.NotEqual(t, first.token, jobs[0].token)
}

// spec: TAG-002
func TestTick_NotDueKeepsJobWithoutRequests(t *testing.T) {
	t.Parallel()

	s := newStand(t, newMemStorage(chat.TaggerUser{ChatID: groupA, UserID: 1, Author: "@alice"}))
	before := s.jobsOf(groupA)

	s.h.tick(context.Background(), time.Now())

	assert.Empty(t, s.api.memberChecks())
	assert.Empty(t, s.api.sentMessages())
	assert.Equal(t, before, s.jobsOf(groupA))
}

// spec: TAG-003
// spec: TAG-024
func TestOnLeft_EmptiedChatStopsAndRefillsWithOneJob(t *testing.T) {
	t.Parallel()

	st := newMemStorage(chat.TaggerUser{ChatID: groupA, UserID: 1, Author: "@alice"})
	s := newStand(t, st)

	require.NoError(t, s.h.OnLeft().Handle(s.left(nil, user(1, "alice"))))
	assert.Empty(t, s.jobsOf(groupA))
	s.h.tick(context.Background(), due())
	assert.Empty(t, s.api.sentMessages(), "без кандидатов пинга нет")

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := int64(10 + i%5)
			assert.NoError(t, s.h.Handle(s.message(groupA, user(id, fmt.Sprintf("u%d", id)))))
		}()
	}
	wg.Wait()

	assert.Len(t, s.candidates(groupA), 5, "без дубликатов участников")
	assert.Len(t, st.users(), 5)
	assert.Len(t, s.jobsOf(groupA), 1, "ровно один следующий пинг")
}

// spec: TAG-024
func TestTick_RefillDuringAttemptKeepsSingleJob(t *testing.T) {
	t.Parallel()

	st := newMemStorage(chat.TaggerUser{ChatID: groupA, UserID: 1, Author: "@alice"})
	s := newStand(t, st)
	s.gen.onGenerate = func() {
		// за время генерации адресат вышел, а в чат пришёл новый участник
		assert.NoError(t, s.h.OnLeft().Handle(s.left(nil, user(1, "alice"))))
		assert.NoError(t, s.h.OnJoin().Handle(s.joined(groupA, nil, user(2, "bob"))))
	}

	s.h.tick(context.Background(), due())

	assert.Empty(t, s.api.sentMessages(), "вышедшему во время генерации пинг не уходит")
	assert.Equal(t, map[int64]string{2: "@bob"}, s.candidates(groupA))
	assert.Len(t, s.jobsOf(groupA), 1, "старая попытка не добавляет вторую задачу")
}

func TestTick_LeftDuringGenerationOfLastCandidateLeavesNoJob(t *testing.T) {
	t.Parallel()

	s := newStand(t, newMemStorage(chat.TaggerUser{ChatID: groupA, UserID: 1, Author: "@alice"}))
	s.gen.onGenerate = func() {
		assert.NoError(t, s.h.OnLeft().Handle(s.left(nil, user(1, "alice"))))
	}

	s.h.tick(context.Background(), due())

	assert.Empty(t, s.api.sentMessages())
	assert.Empty(t, s.jobsOf(groupA), "очищенная задача не возвращается")
}

func TestTick_NicknameOrSendFailureReschedules(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		setup   func(*stand)
		wantLog string
	}{
		"ошибка кличек": {
			setup:   func(s *stand) { s.nicks.err = errDisk },
			wantLog: "can't get nicknames from repositody",
		},
		"пустой пул кличек": {
			setup:   func(s *stand) { s.nicks.list = nil },
			wantLog: "no enabled nicknames for tagger",
		},
		"ошибка отправки": {
			setup:   func(s *stand) { s.api.failSend() },
			wantLog: "can't send tagger message",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			st := newMemStorage(chat.TaggerUser{ChatID: groupA, UserID: 1, Author: "@alice"})
			s := newStand(t, st)
			tc.setup(s)

			s.h.tick(context.Background(), due())

			assert.Equal(t, []string{tc.wantLog}, s.log.messages())
			assert.Equal(t, map[int64]string{1: "@alice"}, s.candidates(groupA))
			assert.Len(t, s.jobsOf(groupA), 1)
		})
	}
}
