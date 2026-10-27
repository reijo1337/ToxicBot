package message

import (
	"fmt"
	"strings"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
)

const (
	maxReactionAuthorRunes = 64
	maxReactionValueRunes  = 128
	maxReactionListValues  = 20
)

func formatReaction(
	entry chathistory.Entry,
	history []chathistory.Entry,
	assistantTurn int,
) string {
	reaction := entry.Reaction.ResolveTarget(history)

	var b strings.Builder
	fmt.Fprintf(
		&b,
		`<reaction update_id="%d" message_id="%d" actor_id="%d" actor="%s" time="%s" target_known="%t"`,
		reaction.UpdateID,
		reaction.MessageID,
		reaction.ActorID,
		reactionAttr(reaction.Actor),
		entry.Time.UTC().Format(timeLayoutLLM),
		reaction.TargetKnown,
	)
	if reaction.TargetKnown {
		fmt.Fprintf(&b, ` target_sender_id="%d" target_author="%s" target_from_bot="%t"`,
			reaction.TargetSenderID, reactionAttr(reaction.TargetAuthor), reaction.TargetFromBot)
	}

	if assistantTurn > 0 {
		fmt.Fprintf(&b, ` target_assistant_turn="%d"`, assistantTurn)
	}

	b.WriteByte('>')
	writeReactionValues(&b, "added", reaction.Added)
	writeReactionValues(&b, "removed", reaction.Removed)
	b.WriteString("</reaction>")

	return b.String()
}

func writeReactionValues(b *strings.Builder, section string, values []chathistory.ReactionType) {
	b.WriteByte('<')
	b.WriteString(section)
	b.WriteByte('>')

	for _, value := range values[:min(len(values), maxReactionListValues)] {
		fmt.Fprintf(b, `<value type="%s">%s</value>`,
			sanitizeAttr(SanitizeText(value.Type, maxReactionValueRunes)),
			SanitizeText(value.Value, maxReactionValueRunes))
	}

	if len(values) > maxReactionListValues {
		b.WriteString("<truncated/>")
	}

	b.WriteString("</")
	b.WriteString(section)
	b.WriteByte('>')
}

func reactionAttr(value string) string {
	return sanitizeAttr(SanitizeText(value, maxReactionAuthorRunes))
}
