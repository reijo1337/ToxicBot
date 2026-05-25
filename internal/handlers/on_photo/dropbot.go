package on_photo

import "github.com/reijo1337/ToxicBot/internal/features/chathistory"

// dropBotEntries returns a new slice containing only user-authored history
// entries. The bot's own past replies (FromBot == true) are removed so the
// model does not imitate them as few-shot examples — that self-imitation is
// what snowballs the repeated photo template («О, "кличка" вылез…»). The input
// slice is never mutated.
func dropBotEntries(history []chathistory.Entry) []chathistory.Entry {
	out := make([]chathistory.Entry, 0, len(history))
	for _, e := range history {
		if e.FromBot {
			continue
		}
		out = append(out, e)
	}
	return out
}
