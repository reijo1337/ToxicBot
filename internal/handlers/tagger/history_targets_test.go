package tagger

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/reijo1337/ToxicBot/internal/domain/chat"
	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/reijo1337/ToxicBot/internal/features/message"
	pkglogger "github.com/reijo1337/ToxicBot/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func historyStand(t *testing.T, users ...chat.TaggerUser) *stand {
	t.Helper()
	s := newStand(t, newMemStorage(users...))
	s.h.log = pkglogger.New()
	return s
}

// spec: TAG-013
// spec: TAG-032
// spec: TAG-033
func TestTick_NoEligibleHistoryAuthorSkipsAndReschedules(t *testing.T) {
	t.Parallel()
	cases := map[string][]chathistory.Entry{
		"empty":                   nil,
		"bot":                     {{SenderID: 1, FromBot: true, Text: "bot"}},
		"legacy matching name":    {{Author: "@same", Text: "legacy"}},
		"unknown negative sender": {{SenderID: -1, Author: "@same", Text: "channel"}},
		"author outside registry": {{SenderID: 99, Author: "@same", Text: "other"}},
	}
	for name, history := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, chance := range []float32{0, 1} {
				t.Run(strconv.FormatFloat(float64(chance), 'f', 0, 32), func(t *testing.T) {
					t.Parallel()
					s := historyStand(
						t,
						chat.TaggerUser{ChatID: groupA, UserID: 1, Author: "@same"},
					)
					s.settings.aiChance = chance
					first := s.jobsOf(groupA)[0]
					buf := NewMockhistoryBuffer(gomock.NewController(t))
					buf.EXPECT().Get(groupA).Return(history).Times(1)
					s.h.history = buf

					s.h.tick(context.Background(), due())

					assert.Empty(t, s.api.memberChecks())
					assert.Empty(t, s.gen.aiChances)
					assert.Empty(t, s.api.sentMessages())
					assert.Equal(t, map[int64]string{1: "@same"}, s.candidates(groupA))
					jobs := s.jobsOf(groupA)
					require.Len(t, jobs, 1)
					assert.NotEqual(t, first.token, jobs[0].token)
					assert.True(t, jobs[0].tagAt.After(time.Now().Add(tagFrom-time.Minute)))
					assert.True(t, jobs[0].tagAt.Before(time.Now().Add(tagTo+time.Minute)))
				})
			}
		})
	}
}

// spec: TAG-032
// spec: TAG-035
func TestTick_SelectionContainsEachEligibleRegistryAuthorOnce(t *testing.T) {
	t.Parallel()
	for index := range 2 {
		t.Run(strconv.Itoa(index), func(t *testing.T) {
			t.Parallel()
			s := historyStand(t,
				chat.TaggerUser{ChatID: groupA, UserID: 1, Author: "@same"},
				chat.TaggerUser{ChatID: groupA, UserID: 2, Author: "@same"},
				chat.TaggerUser{ChatID: groupA, UserID: 3, Author: "@third"},
				chat.TaggerUser{ChatID: groupA, UserID: 4, Author: "@bot"},
			)
			history := []chathistory.Entry{
				{SenderID: 2, Text: "first"},
				{SenderID: 2, Text: "second"},
				{SenderID: 2, Text: "third"},
				{SenderID: 3, Text: "once"},
				{SenderID: 4, FromBot: true},
				{SenderID: 99, Text: "outside"},
				{Author: "@same", Text: "legacy"},
				{SenderID: -1, Text: "channel"},
			}
			ctrl := gomock.NewController(t)
			buf := NewMockhistoryBuffer(ctrl)
			buf.EXPECT().Get(groupA).Return(history).Times(1)
			s.h.history = buf
			random := NewMockrandomizer(ctrl)
			gomock.InOrder(
				random.EXPECT().Intn(2).Return(index),
				random.EXPECT().Intn(1).Return(0),
				random.EXPECT().Int63n(s.h.nextInterval).Return(int64(0)),
			)
			s.h.random = random

			s.h.tick(context.Background(), due())

			assert.Equal(t, []string{strconv.Itoa(index + 2)}, s.api.memberChecks())
			require.Len(t, s.api.sentMessages(), 1)
			assert.Len(t, s.candidates(groupA), 4)
		})
	}
}

// spec: TAG-034
func TestTick_UsesOneSnapshotDespiteHistoryChangeDuringMembership(t *testing.T) {
	t.Parallel()
	s := historyStand(t, chat.TaggerUser{ChatID: groupA, UserID: 1, Author: "@current"})
	s.history = chathistory.NewBuffer(1)
	s.history.Add(groupA, chathistory.Entry{SenderID: 1, Author: "@old", Text: "own snapshot"})
	snapshot := s.history.Get(groupA)
	ctrl := gomock.NewController(t)
	buf := NewMockhistoryBuffer(ctrl)
	buf.EXPECT().Get(groupA).DoAndReturn(func(id int64) []chathistory.Entry {
		return s.history.Get(id)
	}).Times(1)
	s.h.history = buf
	gen := NewMockmessageGenerator(ctrl)
	gen.EXPECT().GetMessageTextForTag(gomock.Any(), snapshot, float32(0),
		message.TagTarget{UserID: 1, Author: "@current", Nickname: nick}).
		Return(message.GenerationResult{Message: "готовая фраза"})
	s.h.generator = gen
	started, release := s.api.holdMember(1)
	done := make(chan struct{})
	go func() { defer close(done); s.h.tick(context.Background(), due()) }()
	<-started
	// The new message evicts the recipient from the current buffer, not from this attempt.
	s.history.Add(groupA, chathistory.Entry{SenderID: 2, Text: "new background"})
	release()
	<-done
	require.Len(t, s.api.sentMessages(), 1)
}

// spec: TAG-034
func TestTick_HistoryLoadDoesNotHoldMutexAndRejectsReplacedJob(t *testing.T) {
	t.Parallel()
	s := historyStand(t, chat.TaggerUser{ChatID: groupA, UserID: 1, Author: "@alice"})
	first := s.jobsOf(groupA)[0]
	ctrl := gomock.NewController(t)
	buf := NewMockhistoryBuffer(ctrl)
	buf.EXPECT().Get(groupA).DoAndReturn(func(int64) []chathistory.Entry {
		unlocked := s.h.mu.TryLock()
		require.True(t, unlocked, "history lazy load must run outside Handler.mu")
		s.h.mu.Unlock()
		require.NoError(t, s.h.forgetChat(context.Background(), groupA))
		require.NoError(t, s.h.addUser(context.Background(), groupA, user(1, "alice")))
		return []chathistory.Entry{{SenderID: 1, Text: "own"}}
	}).Times(1)
	s.h.history = buf

	s.h.tick(context.Background(), due())

	assert.Empty(t, s.api.memberChecks())
	assert.Empty(t, s.gen.aiChances)
	assert.Empty(t, s.api.sentMessages())
	jobs := s.jobsOf(groupA)
	require.Len(t, jobs, 1)
	assert.NotEqual(t, first.token, jobs[0].token)
}

// spec: TAG-025
func TestTick_DisabledChatDoesNotReadHistory(t *testing.T) {
	t.Parallel()
	for _, id := range []int64{groupA, groupB} {
		t.Run(strconv.FormatInt(id, 10), func(t *testing.T) {
			t.Parallel()
			s := historyStand(t, chat.TaggerUser{ChatID: id, UserID: 1, Author: "@alice"})
			s.settings.setEnabled(id, false)
			s.h.history = NewMockhistoryBuffer(gomock.NewController(t))
			s.h.tick(context.Background(), due())
			assert.Empty(t, s.api.memberChecks())
			assert.Empty(t, s.gen.aiChances)
			assert.Empty(t, s.api.sentMessages())
			assert.Len(t, s.jobsOf(id), 1)
		})
	}
}
