package tagger

import (
	"context"
	"sync"
	"testing"

	"github.com/reijo1337/ToxicBot/internal/domain/chat"
	"github.com/reijo1337/ToxicBot/internal/handlers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/telebot.v3"
)

// spec: TAG-018
func TestObserve_AnswerDelayedPastBotRemovalIsDropped(t *testing.T) {
	t.Parallel()

	st := newMemStorage(chat.TaggerUser{ChatID: groupA, UserID: 1, Author: "@alice"})
	s := newStand(t, st)
	started, release := s.api.holdMember(1)

	done := make(chan error, 1)
	go func() { done <- s.h.Handle(s.message(groupA, user(1, "alice"))) }()
	<-started

	bot := &telebot.User{ID: botID, IsBot: true}
	require.NoError(t, s.h.OnLeft().Handle(s.left(nil, bot)))
	release()
	require.NoError(t, <-done)

	assert.Empty(t, st.users(), "старый ответ не возвращает удалённый чат")
	assert.Empty(t, s.candidates(groupA))
	assert.Empty(t, s.jobsOf(groupA))
}

// spec: TAG-008
func TestObserve_AnswerDelayedPastLeaveIsDropped(t *testing.T) {
	t.Parallel()

	st := newMemStorage()
	s := newStand(t, st)
	started, release := s.api.holdMember(5)

	done := make(chan error, 1)
	go func() { done <- s.h.Handle(s.message(groupA, user(5, "eve"))) }()
	<-started

	require.NoError(t, s.h.OnLeft().Handle(s.left(nil, user(5, "eve"))))
	release()
	require.NoError(t, <-done)

	assert.Empty(t, st.users(), "вышедший во время проверки не попадает в кандидаты")
	assert.Empty(t, s.jobsOf(groupA))
}

// spec: TAG-023
func TestOnJoin_BatchJoinThroughDispatcherKeepsEveryone(t *testing.T) {
	t.Parallel()

	st := newMemStorage()
	s := newStand(t, st)

	// Telebot вызывает обработчик по разу на вступившего; задержка гарантирует, что
	// оба вызова читают сообщение уже после всех подмен UserJoined.
	release := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	dispatcher := handlers.New(telebot.OnUserJoined, s.h.OnJoin())
	s.bot.Handle(telebot.OnUserJoined, func(c telebot.Context) error {
		defer wg.Done()
		<-release
		return dispatcher.Handle(c)
	})

	s.bot.ProcessUpdate(telebot.Update{Message: &telebot.Message{
		Chat:        &telebot.Chat{ID: groupA, Type: telebot.ChatSuperGroup},
		UsersJoined: []telebot.User{*user(1, "alice"), *user(2, "bob")},
	}})
	close(release)
	wg.Wait()

	assert.Equal(t, map[int64]string{1: "@alice", 2: "@bob"}, s.candidates(groupA))
	assert.Len(t, st.users(), 2)
	assert.Len(t, s.jobsOf(groupA), 1)
}

// spec: TAG-024
func TestTick_StaleAttemptDoesNotPingRefilledChat(t *testing.T) {
	t.Parallel()

	s := newStand(t, newMemStorage(chat.TaggerUser{ChatID: groupA, UserID: 1, Author: "@alice"}))
	s.gen.onGenerate = func() {
		require.NoError(t, s.h.OnLeft().Handle(s.left(nil, user(1, "alice"))))
		require.NoError(t, s.h.OnJoin().Handle(s.joined(groupA, nil, user(1, "alice"))))
	}

	s.h.tick(context.Background(), due())

	assert.Empty(t, s.api.sentMessages(), "старая попытка не опережает новый срок")
	assert.Len(t, s.jobsOf(groupA), 1)
}

// spec: TAG-024
func TestTick_StaleGoneAnswerKeepsRejoinedUser(t *testing.T) {
	t.Parallel()

	cases := map[string][]chat.TaggerUser{
		"чат опустел": {
			{ChatID: groupA, UserID: 1, Author: "@alice"},
		},
		"в чате остались другие": {
			{ChatID: groupA, UserID: 1, Author: "@alice"},
			{ChatID: groupA, UserID: 2, Author: "@bob"},
		},
	}
	for name, users := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			st := newMemStorage(users...)
			s := newStand(t, st)
			s.api.setMember(1, leftRaw)
			started, release := s.api.holdMember(1)

			done := make(chan struct{})
			go func() {
				defer close(done)
				s.h.tick(context.Background(), due())
			}()
			<-started

			require.NoError(t, s.h.OnLeft().Handle(s.left(nil, user(1, "alice"))))
			require.NoError(t, s.h.OnJoin().Handle(s.joined(groupA, nil, user(1, "alice"))))
			release()
			<-done

			assert.Equal(t, users, st.users(), "старый ответ не удаляет вернувшегося")
			assert.Contains(t, s.candidates(groupA), int64(1))
			assert.Empty(t, s.api.sentMessages())
			assert.Len(t, s.jobsOf(groupA), 1)
		})
	}
}

// spec: TAG-023
func TestTick_StaleGoneAnswerKeepsUserWhoseLeaveWasMissed(t *testing.T) {
	t.Parallel()

	// Выход пропущен, пока бот лежал: запись осталась, пришло только новое вступление.
	users := []chat.TaggerUser{{ChatID: groupA, UserID: 1, Author: "@alice"}}
	st := newMemStorage(users...)
	s := newStand(t, st)
	s.api.setMember(1, leftRaw)
	started, release := s.api.holdMember(1)

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.h.tick(context.Background(), due())
	}()
	<-started

	require.NoError(t, s.h.OnJoin().Handle(s.joined(groupA, nil, user(1, "alice"))))
	release()
	<-done

	assert.Equal(t, users, st.users(), "старый ответ не удаляет вернувшегося")
	assert.Equal(t, map[int64]string{1: "@alice"}, s.candidates(groupA))
	assert.Len(t, s.jobsOf(groupA), 1)
}
