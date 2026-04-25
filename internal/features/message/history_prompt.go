package message

import (
	"strings"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/reijo1337/ToxicBot/internal/infrastructure/ai/deepseek"
)

const maxEntryRunes = 500

// formatUserContent renders a user-side history entry as a single XML-like
// element so the model can tell apart conversation rows even when their
// content contains adversarial text. The body is sanitized to defang any
// nested tag forging or control characters.
func formatUserContent(e chathistory.Entry, history []chathistory.Entry) string {
	var b strings.Builder
	b.WriteString(`<msg time="`)
	b.WriteString(e.Time.Format("15:04"))
	b.WriteString(`" from="`)
	b.WriteString(sanitizeAttr(e.Author))
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
// value cannot break out of an XML attribute. The author values we hand to
// this helper have already been normalized by SanitizeAuthor at the handler
// boundary, so no further heavy filtering is needed here.
func sanitizeAttr(s string) string {
	if !strings.ContainsRune(s, '"') {
		return s
	}
	return strings.ReplaceAll(s, `"`, "")
}

// buildChatCompletions assembles messages for DeepSeek: one system prompt
// followed by each entry from history in chronological order. Bot entries
// become role=assistant; user entries become role=user wrapped in an
// <msg>...</msg> tag. The trigger message is expected to already be the last
// element of history (handlers add it before calling).
func buildChatCompletions(
	system string,
	history []chathistory.Entry,
) []deepseek.ChatMessage {
	msgs := make([]deepseek.ChatMessage, 0, len(history)+1)
	msgs = append(msgs, deepseek.ChatMessage{
		Role:    deepseek.RoleSystem,
		Content: system,
	})

	for _, e := range history {
		if e.FromBot {
			msgs = append(msgs, deepseek.ChatMessage{
				Role:    deepseek.RoleAssistant,
				Content: SanitizeText(e.Text, maxEntryRunes),
			})
			continue
		}
		msgs = append(msgs, deepseek.ChatMessage{
			Role:    deepseek.RoleUser,
			Content: formatUserContent(e, history),
		})
	}

	return msgs
}
