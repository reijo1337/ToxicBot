package cases

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	alice = "@alice"
	bot   = "бот"
	hello = "привет"
)

func at(sec int) time.Time {
	return time.Date(2026, 9, 1, 12, 0, sec, 0, time.UTC)
}

func TestFromHistory_EachBotReplyBecomesCase(t *testing.T) {
	t.Parallel()

	entries := []chathistory.Entry{
		{ID: 1, Time: at(0), Author: alice, Text: hello},
		{ID: 2, Time: at(1), Author: "@bob", Text: "как дела"},
		{ID: 3, Time: at(2), Author: bot, Text: "отвали", FromBot: true},
		{ID: 4, Time: at(3), Author: alice, Text: "грубо"},
		{ID: 5, Time: at(4), Author: bot, Text: "и что", FromBot: true},
	}

	cs := FromHistory(-100, entries, 1)

	require.Len(t, cs, 2)

	assert.Equal(t, "-100-3", cs[0].ID)
	assert.Equal(t, int64(-100), cs[0].ChatID)
	assert.Equal(t, KindText, cs[0].Kind)
	assert.Equal(t, "отвали", cs[0].Actual)
	require.Len(t, cs[0].History, 2)
	trigger, ok := cs[0].Trigger()
	require.True(t, ok)
	assert.Equal(t, "как дела", trigger.Text)

	assert.Equal(t, "-100-5", cs[1].ID)
	assert.Equal(t, "и что", cs[1].Actual)
	require.Len(t, cs[1].History, 4)
	trigger, ok = cs[1].Trigger()
	require.True(t, ok)
	assert.Equal(t, "грубо", trigger.Text)
	assert.True(t, cs[1].History[2].FromBot, "прошлые ответы бота остаются в окне")
}

func TestFromHistory_SortsByTimeThenID(t *testing.T) {
	t.Parallel()

	entries := []chathistory.Entry{
		{ID: 3, Time: at(2), Author: bot, Text: "ответ", FromBot: true},
		{ID: 2, Time: at(1), Author: "@bob", Text: "второе"},
		{ID: 1, Time: at(0), Author: alice, Text: "первое"},
	}

	cs := FromHistory(1, entries, 1)

	require.Len(t, cs, 1)
	assert.Equal(t, "первое", cs[0].History[0].Text)
	assert.Equal(t, "второе", cs[0].History[1].Text)
}

func TestFromHistory_SkipsTooShortAndBotOnlyWindows(t *testing.T) {
	t.Parallel()

	entries := []chathistory.Entry{
		{ID: 1, Time: at(0), Author: bot, Text: "сам с собой", FromBot: true},
		{ID: 2, Time: at(1), Author: bot, Text: "и опять", FromBot: true},
		{ID: 3, Time: at(2), Author: alice, Text: "эй"},
		{ID: 4, Time: at(3), Author: bot, Text: "чего", FromBot: true},
	}

	assert.Empty(t, FromHistory(1, entries, 5), "minHistory режет короткие окна")

	cs := FromHistory(1, entries, 1)
	require.Len(t, cs, 1, "окно без реплик людей кейсом не становится")
	assert.Equal(t, "1-4", cs[0].ID)
}

func TestFromHistory_PhotoTriggerIsPhotoKind(t *testing.T) {
	t.Parallel()

	entries := []chathistory.Entry{
		{
			ID:           1,
			Time:         at(0),
			Author:       alice,
			Text:         "<photo><caption>кот</caption></photo>",
			PreFormatted: true,
		},
		{ID: 2, Time: at(1), Author: bot, Text: "ну и кот", FromBot: true},
	}

	cs := FromHistory(1, entries, 1)

	require.Len(t, cs, 1)
	assert.Equal(t, KindPhoto, cs[0].Kind)
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "cases.json")
	in := []Case{{
		ID:     "1-2",
		ChatID: 1,
		Kind:   KindText,
		History: []Entry{
			{ID: 1, Time: at(0), Author: alice, Text: hello, ReplyToID: 0},
		},
		Actual: "отвали",
	}}

	require.NoError(t, Save(path, in))
	out, err := Load(path)
	require.NoError(t, err)

	assert.Equal(t, in, out)

	domain := out[0].DomainHistory()
	require.Len(t, domain, 1)
	assert.Equal(t, alice, domain[0].Author)
	assert.True(t, domain[0].Time.Equal(at(0)))
}
