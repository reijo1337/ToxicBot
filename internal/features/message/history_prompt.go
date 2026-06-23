package message

import (
	"sort"
	"strings"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
)

const (
	maxEntryRunes = 500
	timeLayoutLLM = "2006-01-02T15:04"
)

// formatUserContent рендерит одну user-запись из истории в виде
// `<msg from="@..." time="..." [reply_to="@..."] [now="true"]>текст</msg>`.
// Авторство теперь явно передаётся через атрибут from= внутри тела сообщения;
// LLMMessage.Name при этом сохраняется. Тело санируется для защиты от подделки
// тегов, если только запись не помечена как PreFormatted (уже-XML от photo-хендлера).
//
// Параметр isTrigger=true добавляет атрибут now="true" — маркер единственной
// реплики, на которую бот должен отвечать; все остальные записи — только контекст.
//
// NOTE: только user-записи оборачиваются в <msg>. Bot-записи (FromBot=true)
// выдаются как bare sanitized text в buildChatCompletions, чтобы модель не
// начала воспроизводить обёртку в своих ответах.
func formatUserContent(e chathistory.Entry, history []chathistory.Entry, isTrigger bool) string {
	var b strings.Builder
	b.WriteString(`<msg from="`)
	b.WriteString(sanitizeAttr(e.Author))
	b.WriteString(`" time="`)
	// timestamp всегда в UTC, чтобы промпт не зависел от TZ хоста.
	b.WriteString(e.Time.UTC().Format(timeLayoutLLM))
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

	// now="true" помечает единственную реплику, на которую бот должен отвечать;
	// всё остальное — контекст.
	if isTrigger {
		b.WriteString(` now="true"`)
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
//	[ system, ...entries ]
//
// User-entries (FromBot=false) are wrapped in `<msg ...>...</msg>` via
// formatUserContent. Bot-entries (FromBot=true) are emitted as BARE sanitized
// text — this asymmetry is intentional: wrapping past assistant turns in the
// same envelope teaches the model "my output looks like that" and it starts
// echoing the wrapper back in its replies. Authorship is carried in the
// LLMMessage.Name field for both roles. Leading assistant entries (history
// that begins with bot output, e.g. tagger-initiated reply after a restart)
// are skipped — OpenAI-compatible providers do not handle a
// system→assistant→... sequence well.
func buildChatCompletions(
	system string,
	history []chathistory.Entry,
) []LLMMessage {
	// The buffer preserves Add/AddAll insertion order, which diverges from
	// chronological order: telebot processes updates concurrently and the bulling
	// /photo handlers defer AddAll until after a slow LLM call. Sort by Time so the
	// model sees a coherent transcript; ID (monotonic per chat) breaks same-second
	// ties since Telegram timestamps have only second resolution.
	sorted := make([]chathistory.Entry, len(history))
	copy(sorted, history)
	sort.SliceStable(sorted, func(i, j int) bool {
		if !sorted[i].Time.Equal(sorted[j].Time) {
			return sorted[i].Time.Before(sorted[j].Time)
		}
		return sorted[i].ID < sorted[j].ID
	})

	// Самая поздняя user-реплика — та, на которую бот отвечает; остальное контекст.
	triggerIdx := -1
	for i := len(sorted) - 1; i >= 0; i-- {
		if !sorted[i].FromBot {
			triggerIdx = i
			break
		}
	}

	msgs := make([]LLMMessage, 0, len(sorted)+1)
	msgs = append(msgs, LLMMessage{Role: RoleSystem, Content: system})

	skipping := true
	for i, e := range sorted {
		if skipping && e.FromBot {
			continue
		}
		skipping = false

		if e.FromBot {
			msgs = append(msgs, LLMMessage{
				Role:    RoleAssistant,
				Name:    e.Author,
				Content: SanitizeText(e.Text, maxEntryRunes),
			})
			continue
		}

		msgs = append(msgs, LLMMessage{
			Role:    RoleUser,
			Name:    e.Author,
			Content: formatUserContent(e, sorted, i == triggerIdx),
		})
	}

	return msgs
}
