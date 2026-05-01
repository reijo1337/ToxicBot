package message

import (
	"strings"
	"testing"
	"time"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatUserContent_NoReplyTo(t *testing.T) {
	t.Parallel()
	t1 := time.Date(2026, 5, 1, 12, 34, 0, 0, time.UTC)
	e := chathistory.Entry{ID: 1, Time: t1, Author: "@alice", Text: "hello"}
	got := formatUserContent(e, []chathistory.Entry{e})
	assert.Equal(t, `<msg time="2026-05-01T12:34">hello</msg>`, got)
}

func TestFormatUserContent_ReplyToPresent(t *testing.T) {
	t.Parallel()
	t1 := time.Date(2026, 5, 1, 12, 34, 0, 0, time.UTC)
	target := chathistory.Entry{ID: 1, Time: t1, Author: "@bob", Text: "first"}
	e := chathistory.Entry{
		ID:        2,
		Time:      t1.Add(time.Minute),
		Author:    "@alice",
		Text:      "reply",
		ReplyToID: 1,
	}
	got := formatUserContent(e, []chathistory.Entry{target, e})
	assert.Equal(t, `<msg time="2026-05-01T12:35" reply_to="@bob">reply</msg>`, got)
}

func TestFormatUserContent_ReplyToEvicted_NoArrow(t *testing.T) {
	t.Parallel()
	t1 := time.Date(2026, 5, 1, 12, 34, 0, 0, time.UTC)
	e := chathistory.Entry{ID: 2, Time: t1, Author: "@alice", Text: "reply", ReplyToID: 9999}
	got := formatUserContent(e, []chathistory.Entry{e})
	assert.Equal(t, `<msg time="2026-05-01T12:34">reply</msg>`, got,
		"reply_to attribute must be omitted when target ID is not in history")
}

func TestFormatUserContent_NewlineInTextBecomesSpace(t *testing.T) {
	t.Parallel()
	t1 := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	e := chathistory.Entry{ID: 1, Time: t1, Author: "@alice", Text: "line1\nline2"}
	got := formatUserContent(e, []chathistory.Entry{e})
	assert.Equal(t, `<msg time="2026-05-01T00:00">line1 line2</msg>`, got)
}

func TestFormatUserContent_AngleBracketsEscaped(t *testing.T) {
	t.Parallel()
	t1 := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	e := chathistory.Entry{ID: 1, Time: t1, Author: "@alice", Text: "<b>oh hi</b>"}
	got := formatUserContent(e, []chathistory.Entry{e})
	// SanitizeText converts angle brackets to guillemets so the text cannot
	// forge an opening or closing tag.
	assert.NotContains(t, got, "<b>")
	assert.NotContains(t, got, "</b>")
}

func TestFormatUserContent_ReplyToAttributeQuoteStripped(t *testing.T) {
	t.Parallel()
	t1 := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	target := chathistory.Entry{ID: 1, Time: t1, Author: `@bob"injected="x`, Text: "first"}
	e := chathistory.Entry{
		ID:        2,
		Time:      t1.Add(time.Minute),
		Author:    "@alice",
		Text:      "reply",
		ReplyToID: 1,
	}
	got := formatUserContent(e, []chathistory.Entry{target, e})
	assert.Equal(t, `<msg time="2026-05-01T00:01" reply_to="@bobinjected=x">reply</msg>`, got,
		"double-quotes in resolved reply_to author must be stripped")
}

func TestFormatUserContent_LongTextTruncated(t *testing.T) {
	t.Parallel()
	t1 := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	long := strings.Repeat("a", 600)
	e := chathistory.Entry{ID: 1, Time: t1, Author: "@alice", Text: long}
	got := formatUserContent(e, []chathistory.Entry{e})
	bodyStart := len(`<msg time="2026-05-01T00:00">`)
	bodyEnd := len(got) - len(`</msg>`)
	body := got[bodyStart:bodyEnd]
	assert.Len(t, []rune(body), maxEntryRunes,
		"body must be truncated to maxEntryRunes runes")
}

func TestFormatUserContent_PreFormatted_KeepsTags(t *testing.T) {
	t.Parallel()
	t1 := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	body := `<photo><caption>nice</caption><vision_description>cat</vision_description></photo>`
	e := chathistory.Entry{ID: 1, Time: t1, Author: "@alice", Text: body, PreFormatted: true}
	got := formatUserContent(e, []chathistory.Entry{e})
	assert.Equal(t, `<msg time="2026-05-01T00:00">`+body+`</msg>`, got,
		"PreFormatted=true must skip body sanitization")
}

func TestFormatUserContent_NotPreFormatted_StillSanitizes(t *testing.T) {
	t.Parallel()
	t1 := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	body := `<photo><caption>nice</caption></photo>`
	e := chathistory.Entry{ID: 1, Time: t1, Author: "@alice", Text: body, PreFormatted: false}
	got := formatUserContent(e, []chathistory.Entry{e})
	assert.NotContains(t, got, "<photo>",
		"PreFormatted=false must run SanitizeText on the body")
}

func TestBuildChatCompletions_AssemblyOrderAndSystem(t *testing.T) {
	t.Parallel()
	system := "BE TOXIC"
	t1 := time.Date(2026, 5, 1, 14, 32, 0, 0, time.UTC)
	t2 := t1.Add(2 * time.Minute)
	history := []chathistory.Entry{
		{ID: 1, Time: t1, Author: "@alice", Text: "hi", FromBot: false},
		{ID: 2, Time: t2, Author: "@bob", Text: "yo", ReplyToID: 1, FromBot: false},
	}

	msgs := buildChatCompletions(system, history)
	require.Len(t, msgs, 3)

	assert.Equal(t, RoleSystem, msgs[0].Role)
	assert.Equal(t, system, msgs[0].Content)
	assert.Empty(t, msgs[0].Name, "system message must not carry a name")

	assert.Equal(t, RoleUser, msgs[1].Role)
	assert.Equal(t, "@alice", msgs[1].Name)
	assert.Equal(t, `<msg time="2026-05-01T14:32">hi</msg>`, msgs[1].Content)

	assert.Equal(t, RoleUser, msgs[2].Role)
	assert.Equal(t, "@bob", msgs[2].Name)
	assert.Equal(t, `<msg time="2026-05-01T14:34" reply_to="@alice">yo</msg>`, msgs[2].Content)
}

func TestBuildChatCompletions_BotEntrySanitizedToAssistant(t *testing.T) {
	t.Parallel()
	system := "S"
	t1 := time.Date(2026, 5, 1, 14, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Minute)
	t3 := t2.Add(time.Minute)
	history := []chathistory.Entry{
		{ID: 1, Time: t1, Author: "@alice", Text: "hello", FromBot: false},
		{ID: 2, Time: t2, Author: "@toxic_bot", Text: "rude\nreply", FromBot: true, ReplyToID: 1},
		{ID: 3, Time: t3, Author: "@alice", Text: "fuck off", FromBot: false},
	}

	msgs := buildChatCompletions(system, history)
	require.Len(t, msgs, 4)

	assert.Equal(t, RoleAssistant, msgs[2].Role)
	assert.Equal(t, "@toxic_bot", msgs[2].Name)
	assert.Equal(
		t,
		`rude reply`,
		msgs[2].Content,
		"bot entry must be bare sanitized text without <msg> envelope",
	)
}

func TestBuildChatCompletions_BotReplySetsReplyToTagOnNextUser(t *testing.T) {
	t.Parallel()
	system := "S"
	t1 := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	history := []chathistory.Entry{
		{ID: 1, Time: t1, Author: "@alice", Text: "first", FromBot: false},
		{
			ID:        2,
			Time:      t1.Add(time.Minute),
			Author:    "@toxic_bot",
			Text:      "rude",
			ReplyToID: 1,
			FromBot:   true,
		},
		{
			ID:        3,
			Time:      t1.Add(2 * time.Minute),
			Author:    "@alice",
			Text:      "back at you",
			ReplyToID: 2,
			FromBot:   false,
		},
	}

	msgs := buildChatCompletions(system, history)
	require.Len(t, msgs, 4)

	assert.Equal(t, RoleUser, msgs[1].Role)
	assert.Equal(t, RoleAssistant, msgs[2].Role)
	assert.Equal(t, RoleUser, msgs[3].Role)

	assert.Equal(t, `rude`, msgs[2].Content,
		"bot entry must be bare sanitized text without <msg> envelope")
	assert.Equal(
		t,
		`<msg time="2026-05-01T12:02" reply_to="@toxic_bot">back at you</msg>`,
		msgs[3].Content,
	)
}

func TestBuildChatCompletions_SingleUser(t *testing.T) {
	t.Parallel()
	system := "S"
	t1 := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	history := []chathistory.Entry{
		{ID: 7, Time: t1, Author: "@solo", Text: "yo"},
	}
	msgs := buildChatCompletions(system, history)
	require.Len(t, msgs, 2)
	assert.Equal(t, RoleSystem, msgs[0].Role)
	assert.Equal(t, RoleUser, msgs[1].Role)
	assert.Equal(t, "@solo", msgs[1].Name)
	assert.Equal(t, `<msg time="2026-05-01T09:00">yo</msg>`, msgs[1].Content)
}

func TestBuildChatCompletions_LeadingAssistantsAreSkipped(t *testing.T) {
	t.Parallel()
	system := "S"
	t1 := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	history := []chathistory.Entry{
		{ID: 1, Time: t1, Author: "@toxic_bot", Text: "first bot line", FromBot: true},
		{
			ID:      2,
			Time:    t1.Add(time.Minute),
			Author:  "@toxic_bot",
			Text:    "second bot line",
			FromBot: true,
		},
		{ID: 3, Time: t1.Add(2 * time.Minute), Author: "@alice", Text: "hello", FromBot: false},
	}
	msgs := buildChatCompletions(system, history)

	require.Len(t, msgs, 2, "leading assistant entries must be skipped, leaving system + user")
	assert.Equal(t, RoleSystem, msgs[0].Role)
	assert.Equal(t, RoleUser, msgs[1].Role)
	assert.Equal(t, "@alice", msgs[1].Name)
}
