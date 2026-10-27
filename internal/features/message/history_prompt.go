package message

import (
	"fmt"
	"sort"
	"strings"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
)

const (
	maxEntryRunes = 500
	timeLayoutLLM = "2006-01-02T15:04"
)

// formatUserContent рендерит одну user-запись из истории в виде
// `<msg id="..." from="@..." time="..." [reply_to="@..."] [now="true"]>текст</msg>`.
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
	fmt.Fprintf(&b, `<msg id="%d" from="`, e.ID)
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
	writeEntryBody(&b, e)
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
	mode generationMode,
) []LLMMessage {
	sorted := orderedHistory(history)
	entries := withoutLeadingBotEntries(sorted)
	assistantTurns := make(map[int]int)
	if mode == replyGeneration {
		turn := 0

		for _, entry := range entries {
			if entry.Reaction == nil && entry.FromBot {
				turn++
				assistantTurns[entry.ID] = turn
			}
		}
	}

	// В ответе последняя user-реплика — триггер; при пинге вся история — контекст.
	triggerIdx := -1
	if mode == replyGeneration {
		triggerIdx = lastUserMessageIndex(entries)
	}

	msgs := make([]LLMMessage, 0, len(sorted)+1)
	msgs = append(msgs, LLMMessage{
		Role:    RoleSystem,
		Content: system,
	})

	for i, e := range entries {
		if e.Reaction != nil {
			msgs = append(msgs, LLMMessage{
				Role:    RoleUser,
				Content: formatReaction(e, sorted, assistantTurns[e.Reaction.MessageID]),
			})

			continue
		}

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

func withoutLeadingBotEntries(history []chathistory.Entry) []chathistory.Entry {
	entries := make([]chathistory.Entry, 0, len(history))
	skipping := true

	for _, entry := range history {
		if entry.Reaction == nil {
			if skipping && entry.FromBot {
				continue
			}

			skipping = false
		}

		entries = append(entries, entry)
	}

	return entries
}

func orderedHistory(history []chathistory.Entry) []chathistory.Entry {
	sorted := append([]chathistory.Entry(nil), history...)
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if !a.Time.Equal(b.Time) {
			return a.Time.Before(b.Time)
		}

		if a.Reaction == nil && b.Reaction == nil {
			return a.ID < b.ID
		}

		if a.Reaction == nil || b.Reaction == nil {
			return a.Reaction == nil
		}

		return a.Reaction.UpdateID < b.Reaction.UpdateID
	})

	return sorted
}

func lastUserMessageIndex(history []chathistory.Entry) int {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Reaction == nil && !history[i].FromBot {
			return i
		}
	}

	return -1
}
