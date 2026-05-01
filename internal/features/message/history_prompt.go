package message

import (
	"strings"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
)

const (
	maxEntryRunes = 500
	timeLayoutLLM = "2006-01-02T15:04"
)

// formatUserContent renders one history entry as `<msg time="..." [reply_to="@..."]>текст</msg>`.
// The `from=` attribute used to live here too; authorship now travels in
// LLMMessage.Name. The body is sanitized to defang any nested tag forging or
// control characters unless the caller marked the entry as PreFormatted
// (already-XML-formatted bodies produced by the photo handler).
func formatUserContent(e chathistory.Entry, history []chathistory.Entry) string {
	var b strings.Builder
	b.WriteString(`<msg time="`)
	b.WriteString(e.Time.Format(timeLayoutLLM))
	b.WriteString(`"`)

	if e.ReplyToID != 0 {
		for _, h := range history {
			if h.ID == e.ReplyToID {
				b.WriteString(` reply_to="`)
				b.WriteString(sanitizeAttr(h.Author))
				b.WriteString(`"`)
				break
			}
		}
	}

	b.WriteString(`>`)
	if e.PreFormatted {
		b.WriteString(e.Text)
	} else {
		b.WriteString(SanitizeText(e.Text, maxEntryRunes))
	}
	b.WriteString(`</msg>`)

	return b.String()
}

// sanitizeAttr strips the double-quote character so an attacker-controlled
// value cannot break out of an XML attribute.
func sanitizeAttr(s string) string {
	if !strings.ContainsRune(s, '"') {
		return s
	}
	return strings.ReplaceAll(s, `"`, "")
}

// buildChatCompletions produces the message envelope for the LLM:
//
//	[ system, ...wrapped entries ]
//
// Both user-Entry and bot-Entry are wrapped in <msg ...>...</msg>; the only
// difference is Role (User vs Assistant). Authorship is carried in the
// LLMMessage.Name field, not in the envelope. Leading assistant entries
// (history that begins with bot output, e.g. tagger-initiated reply after a
// restart) are skipped — OpenAI-compatible providers do not handle a
// system→assistant→... sequence well.
func buildChatCompletions(
	system string,
	history []chathistory.Entry,
) []LLMMessage {
	msgs := make([]LLMMessage, 0, len(history)+1)
	msgs = append(msgs, LLMMessage{Role: RoleSystem, Content: system})

	skipping := true
	for _, e := range history {
		if skipping && e.FromBot {
			continue
		}
		skipping = false

		role := RoleUser
		if e.FromBot {
			role = RoleAssistant
		}
		msgs = append(msgs, LLMMessage{
			Role:    role,
			Name:    e.Author,
			Content: formatUserContent(e, history),
		})
	}

	return msgs
}
