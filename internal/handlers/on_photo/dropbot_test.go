package on_photo

import (
	"testing"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDropBotEntries_KeepsUsersDropsBotInOrder(t *testing.T) {
	t.Parallel()

	in := []chathistory.Entry{
		{ID: 1, Author: "@bob", Text: "u1", FromBot: false},
		{ID: 2, Author: "@toxic", Text: "b1", FromBot: true},
		{ID: 3, Author: "@alice", Text: "u2", FromBot: false},
		{ID: 4, Author: "@toxic", Text: "b2", FromBot: true},
	}

	got := dropBotEntries(in)

	require.Len(t, got, 2)
	assert.Equal(t, "u1", got[0].Text)
	assert.Equal(t, "u2", got[1].Text)
	for _, e := range got {
		assert.False(t, e.FromBot)
	}
}

func TestDropBotEntries_DoesNotMutateInput(t *testing.T) {
	t.Parallel()

	in := []chathistory.Entry{
		{ID: 1, Text: "u1", FromBot: false},
		{ID: 2, Text: "b1", FromBot: true},
	}

	_ = dropBotEntries(in)

	require.Len(t, in, 2)
	assert.Equal(t, "b1", in[1].Text)
	assert.True(t, in[1].FromBot)
}

func TestDropBotEntries_EmptyAndEdgeCases(t *testing.T) {
	t.Parallel()

	assert.Empty(t, dropBotEntries(nil))
	assert.Empty(t, dropBotEntries([]chathistory.Entry{}))

	allBot := []chathistory.Entry{
		{ID: 1, FromBot: true},
		{ID: 2, FromBot: true},
	}
	assert.Empty(t, dropBotEntries(allBot))

	allUser := []chathistory.Entry{
		{ID: 1, Text: "u1", FromBot: false},
		{ID: 2, Text: "u2", FromBot: false},
	}
	require.Len(t, dropBotEntries(allUser), 2)
}
