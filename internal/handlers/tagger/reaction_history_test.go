package tagger

import (
	"context"
	"testing"

	"github.com/reijo1337/ToxicBot/internal/domain/chat"
	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/stretchr/testify/assert"
)

// spec: GEN-055
// spec: TAG-036
func TestTick_ReactionCannotGrantAdmissionEvenWithSenderID(t *testing.T) {
	t.Parallel()

	s := historyStand(t, chat.TaggerUser{
		ChatID: groupA,
		UserID: 1,
		Author: "@actor",
	})

	s.history.Add(groupA, chathistory.Entry{
		SenderID: 1,
		Reaction: &chathistory.Reaction{
			UpdateID:  900,
			MessageID: 42,
			ActorID:   1,
			Actor:     "@actor",
		},
	})
	s.h.tick(context.Background(), due())

	assert.Empty(t, s.api.memberChecks())
	assert.Empty(t, s.api.sentMessages())
	assert.Equal(t, map[int64]string{1: "@actor"}, s.candidates(groupA))
}

// spec: GEN-055
// spec: TAG-036
func TestTick_ReactionActorDoesNotRegisterCandidate(t *testing.T) {
	t.Parallel()

	s := historyStand(t)
	s.history.Add(groupA, chathistory.Entry{
		Reaction: &chathistory.Reaction{
			UpdateID:  900,
			MessageID: 42,
			ActorID:   1,
			Actor:     "@actor",
		},
	})
	s.h.tick(context.Background(), due())

	assert.Empty(t, s.candidates(groupA))
	assert.Empty(t, s.jobsOf(groupA))
	assert.Empty(t, s.api.memberChecks())
	assert.Empty(t, s.api.sentMessages())
}
