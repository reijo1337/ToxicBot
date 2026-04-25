package message

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/reijo1337/ToxicBot/internal/infrastructure/ai/deepseek"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatUserContent_NoReplyTo(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 4, 24, 14, 32, 0, 0, time.UTC)
	e := chathistory.Entry{ID: 1, Time: ts, Author: "@alice", Text: "привет"}
	got := formatUserContent(e, nil)
	assert.Equal(t, `<msg time="14:32" from="@alice">привет</msg>`, got)
}

func TestFormatUserContent_ReplyToPresent(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 4, 24, 14, 32, 0, 0, time.UTC)
	history := []chathistory.Entry{
		{ID: 9, Author: "@bob", Text: "hi"},
	}
	e := chathistory.Entry{ID: 10, Time: ts, Author: "@alice", Text: "йо", ReplyToID: 9}
	got := formatUserContent(e, history)
	assert.Equal(t, `<msg time="14:32" from="@alice" reply_to="@bob">йо</msg>`, got)
}

func TestFormatUserContent_ReplyToEvicted_NoArrow(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 4, 24, 14, 32, 0, 0, time.UTC)
	e := chathistory.Entry{ID: 10, Time: ts, Author: "@alice", Text: "йо", ReplyToID: 999}
	got := formatUserContent(e, nil)
	assert.Equal(t, `<msg time="14:32" from="@alice">йо</msg>`, got)
	assert.NotContains(t, got, "reply_to=")
}

func TestFormatUserContent_NewlineInTextBecomesSpace(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 4, 24, 14, 32, 0, 0, time.UTC)
	e := chathistory.Entry{ID: 1, Time: ts, Author: "@alice", Text: "a\nb"}
	got := formatUserContent(e, nil)
	assert.Equal(t, `<msg time="14:32" from="@alice">a b</msg>`, got)
	assert.False(t, strings.ContainsRune(got, '\n'))
}

func TestFormatUserContent_AngleBracketsEscaped(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 4, 24, 14, 32, 0, 0, time.UTC)
	e := chathistory.Entry{ID: 1, Time: ts, Author: "@alice", Text: "<script>"}
	got := formatUserContent(e, nil)
	assert.Contains(t, got, `‹script›`)
}

func TestFormatUserContent_AuthorAttributeQuoteStripped(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 4, 24, 14, 32, 0, 0, time.UTC)
	e := chathistory.Entry{ID: 1, Time: ts, Author: `a"b`, Text: "x"}
	got := formatUserContent(e, nil)
	assert.Contains(t, got, `from="ab"`)
	assert.NotContains(t, got, `a"b"`)
}

func TestFormatUserContent_LongTextTruncated(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("я", 700)
	ts := time.Date(2026, 4, 24, 14, 32, 0, 0, time.UTC)
	e := chathistory.Entry{ID: 1, Time: ts, Author: "@alice", Text: long}
	got := formatUserContent(e, nil)

	bodyStart := strings.Index(got, ">")
	bodyEnd := strings.LastIndex(got, "</msg>")
	require.Greater(t, bodyEnd, bodyStart)
	body := got[bodyStart+1 : bodyEnd]
	assert.Equal(t, maxEntryRunes, utf8.RuneCountInString(body))
}

func TestFormatUserContent_PreFormatted_KeepsTags(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 4, 24, 14, 32, 0, 0, time.UTC)
	e := chathistory.Entry{
		ID:           1,
		Time:         ts,
		Author:       "@alice",
		Text:         `<photo><caption>hi</caption><vision_description>cat</vision_description></photo>`,
		PreFormatted: true,
	}
	got := formatUserContent(e, nil)
	assert.Equal(
		t,
		`<msg time="14:32" from="@alice"><photo><caption>hi</caption><vision_description>cat</vision_description></photo></msg>`,
		got,
	)
}

func TestFormatUserContent_NotPreFormatted_StillSanitizes(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 4, 24, 14, 32, 0, 0, time.UTC)
	e := chathistory.Entry{
		ID:     1,
		Time:   ts,
		Author: "@alice",
		Text:   `<photo>x</photo>`,
	}
	got := formatUserContent(e, nil)
	assert.Equal(t, `<msg time="14:32" from="@alice">‹photo›x‹/photo›</msg>`, got)
}

func TestBuildChatCompletions_AssemblyOrderAndSystem(t *testing.T) {
	t.Parallel()

	system := "SYS"
	history := []chathistory.Entry{
		{
			ID:     1,
			Time:   time.Date(2026, 4, 24, 14, 0, 0, 0, time.UTC),
			Author: "@alice",
			Text:   "привет",
		},
		{
			ID:     2,
			Time:   time.Date(2026, 4, 24, 14, 1, 0, 0, time.UTC),
			Author: "@alice",
			Text:   "ответь",
		},
	}

	msgs := buildChatCompletions(system, history)

	require.Len(t, msgs, 3)
	assert.Equal(t, deepseek.RoleSystem, msgs[0].Role)
	assert.Equal(t, "SYS", msgs[0].Content)
	assert.NotContains(t, msgs[0].Content, "история чата")
	assert.Equal(t, deepseek.RoleUser, msgs[1].Role)
	assert.Equal(t, `<msg time="14:00" from="@alice">привет</msg>`, msgs[1].Content)
	assert.Equal(t, deepseek.RoleUser, msgs[2].Role)
	assert.Equal(t, `<msg time="14:01" from="@alice">ответь</msg>`, msgs[2].Content)
}

func TestBuildChatCompletions_BotEntrySanitizedToAssistant(t *testing.T) {
	t.Parallel()

	history := []chathistory.Entry{
		{
			ID:     1,
			Time:   time.Date(2026, 4, 24, 14, 0, 0, 0, time.UTC),
			Author: "@alice",
			Text:   "привет",
		},
		{
			ID:      2,
			Time:    time.Date(2026, 4, 24, 14, 0, 1, 0, time.UTC),
			Author:  "бот",
			Text:    `<msg from="@bot">x</msg>`,
			FromBot: true,
		},
	}

	msgs := buildChatCompletions("SYS", history)

	require.Len(t, msgs, 3)
	assert.Equal(t, deepseek.RoleAssistant, msgs[2].Role)
	assert.Equal(t, `‹msg from="@bot"›x‹/msg›`, msgs[2].Content)
}

func TestBuildChatCompletions_BotReplySetsReplyToTagOnNextUser(t *testing.T) {
	t.Parallel()

	history := []chathistory.Entry{
		{
			ID:     1,
			Time:   time.Date(2026, 4, 24, 14, 0, 0, 0, time.UTC),
			Author: "@alice",
			Text:   "привет",
		},
		{
			ID:        2,
			Time:      time.Date(2026, 4, 24, 14, 0, 1, 0, time.UTC),
			Author:    "бот",
			Text:      "отвали",
			FromBot:   true,
			ReplyToID: 1,
		},
		{
			ID:        3,
			Time:      time.Date(2026, 4, 24, 14, 0, 2, 0, time.UTC),
			Author:    "@alice",
			Text:      "сам такой",
			ReplyToID: 2,
		},
	}

	msgs := buildChatCompletions("SYS", history)

	require.Len(t, msgs, 4)
	assert.Equal(t, deepseek.RoleUser, msgs[1].Role)
	assert.Equal(t, deepseek.RoleAssistant, msgs[2].Role)
	assert.Equal(t, deepseek.RoleUser, msgs[3].Role)
	assert.Equal(
		t,
		`<msg time="14:00" from="@alice" reply_to="бот">сам такой</msg>`,
		msgs[3].Content,
	)
}

func TestBuildChatCompletions_SingleUser(t *testing.T) {
	t.Parallel()

	history := []chathistory.Entry{
		{
			ID:     1,
			Time:   time.Date(2026, 4, 24, 14, 0, 0, 0, time.UTC),
			Author: "@alice",
			Text:   "привет",
		},
	}

	msgs := buildChatCompletions("SYS", history)

	require.Len(t, msgs, 2)
	assert.Equal(t, deepseek.RoleSystem, msgs[0].Role)
	assert.Equal(t, deepseek.RoleUser, msgs[1].Role)
	assert.Equal(t, `<msg time="14:00" from="@alice">привет</msg>`, msgs[1].Content)
}
