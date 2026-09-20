package lab

import (
	"testing"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyBotHistory(t *testing.T) {
	t.Parallel()

	h := []chathistory.Entry{
		{ID: 1, Author: "@a", Text: "раз"},
		{ID: 2, Author: "бот", Text: "два", FromBot: true},
		{ID: 3, Author: "@a", Text: "три"},
		{ID: 4, Author: "бот", Text: "четыре", FromBot: true},
		{ID: 5, Author: "@a", Text: "пять"},
	}

	ids := func(es []chathistory.Entry) []int {
		out := make([]int, 0, len(es))
		for _, e := range es {
			out = append(out, e.ID)
		}
		return out
	}

	assert.Equal(t, []int{1, 2, 3, 4, 5}, ids(applyBotHistory(h, BotHistoryAll)))
	assert.Equal(t, []int{1, 3, 5}, ids(applyBotHistory(h, BotHistoryNone)))
	assert.Equal(t, []int{1, 3, 4, 5}, ids(applyBotHistory(h, BotHistoryLast)))

	_, err := ParseBotHistory("some")
	require.Error(t, err)
}
