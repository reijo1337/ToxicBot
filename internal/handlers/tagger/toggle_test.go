package tagger

import (
	"context"
	"testing"
	"time"

	"github.com/reijo1337/ToxicBot/internal/domain/chat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// spec: TAG-025
// spec: TAG-027
func TestTick_DisabledChatSilentUntilEnabled(t *testing.T) {
	t.Parallel()

	s := newStand(t, newMemStorage(chat.TaggerUser{ChatID: groupA, UserID: 1, Author: "@alice"}))
	s.settings.setEnabled(groupA, false)
	first := s.jobsOf(groupA)[0]

	s.h.tick(context.Background(), due())

	assert.Empty(t, s.api.memberChecks(), "выключенный чат не тратит запросы к Telegram")
	assert.Empty(t, s.api.sentMessages())
	jobs := s.jobsOf(groupA)
	require.Len(t, jobs, 1)
	assert.NotEqual(t, first.token, jobs[0].token)
	assert.True(t, jobs[0].tagAt.After(time.Now().Add(tagFrom-time.Minute)), "обычный интервал")

	s.settings.setEnabled(groupA, true)
	s.h.tick(context.Background(), time.Now().Add(2*tagTo+time.Minute))

	require.Len(t, s.api.sentMessages(), 1, "после включения пинг без новых сообщений")
}

// spec: TAG-026
func TestHandle_DisabledChatKeepsCollecting(t *testing.T) {
	t.Parallel()

	st := newMemStorage(chat.TaggerUser{ChatID: groupA, UserID: 1, Author: "@alice"})
	s := newStand(t, st)
	s.settings.setEnabled(groupA, false)

	require.NoError(t, s.h.Handle(s.message(groupA, user(2, "bob"))))
	require.NoError(t, s.h.OnJoin().Handle(s.joined(groupA, nil, user(3, "carol"))))
	require.NoError(t, s.h.OnLeft().Handle(s.left(nil, user(1, "alice"))))

	assert.Equal(t, map[int64]string{2: "@bob", 3: "@carol"}, s.candidates(groupA))
	assert.Len(t, st.users(), 2)
	assert.Len(t, s.jobsOf(groupA), 1)
}

// spec: TAG-005
// spec: TAG-028
func TestTick_SettingsErrorDoesNotDisable(t *testing.T) {
	t.Parallel()

	s := newStand(t, newMemStorage(chat.TaggerUser{ChatID: groupA, UserID: 1, Author: "@alice"}))
	s.settings.aiChance = 0.7
	s.settings.fail(errDisk)

	s.h.tick(context.Background(), due())

	assert.Len(t, s.api.sentMessages(), 1)
	assert.Equal(t, []float32{0}, s.gen.aiChances, "при ошибке — нулевая вероятность LLM")
	assert.Equal(t, []string{"can't get chat settings for tagger"}, s.log.messages())
}

// spec: TAG-005
func TestTick_AIChanceFromChatSettings(t *testing.T) {
	t.Parallel()

	s := newStand(t, newMemStorage(chat.TaggerUser{ChatID: groupA, UserID: 1, Author: "@alice"}))
	s.settings.aiChance = 0.7

	s.h.tick(context.Background(), due())

	assert.Equal(t, []float32{0.7}, s.gen.aiChances)
}
