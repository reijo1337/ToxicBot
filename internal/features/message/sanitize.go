package message

import (
	"fmt"
	"strings"
	"unicode"
)

// SanitizeText normalizes free-form text taken from Telegram messages so it
// can be safely embedded into the LLM prompt as the inner body of an XML-like
// tag. It strips control / bidi / zero-width characters, replaces line breaks
// and tabs with spaces, escapes angle brackets to single guillemets so
// attackers can't forge tag boundaries, collapses whitespace and truncates
// the result to the requested rune budget.
func SanitizeText(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}

	var b strings.Builder
	b.Grow(len(s))

	for _, r := range s {
		switch r {
		case '\n', '\r', '\t':
			b.WriteRune(' ')
			continue
		case '<':
			b.WriteRune('‹')
			continue
		case '>':
			b.WriteRune('›')
			continue
		}
		if isStripped(r) {
			continue
		}
		b.WriteRune(r)
	}

	collapsed := collapseSpaces(b.String())
	collapsed = strings.TrimSpace(collapsed)

	return truncateRunes(collapsed, maxRunes)
}

// SanitizeAuthor produces a stable, prompt-safe display name for a Telegram
// user. Bots get a constant placeholder; users with a username are addressed
// as @username (Telegram already restricts that alphabet); otherwise the
// first name is filtered down to letters/digits/space/_/- and truncated. If
// nothing usable remains, a numeric fallback is returned.
func SanitizeAuthor(username, firstName string, userID int64, isBot bool) string {
	if isBot {
		return "Админ какого-то канала"
	}

	if username != "" {
		return "@" + username
	}

	var b strings.Builder
	b.Grow(len(firstName))
	for _, r := range firstName {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == ' ' || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}

	cleaned := strings.TrimSpace(b.String())
	cleaned = truncateRunes(cleaned, 32)

	if cleaned == "" {
		return fmt.Sprintf("пользователь #%d", userID)
	}

	return cleaned
}

// StripOutputMsgEnvelope removes a `<msg ...>...</msg>` wrapper that the model
// echoes back at the start of its reply. It strips:
//   - the leading envelope when the trimmed string starts with `<msg`;
//   - any trailing chatter the model appends after the closing `</msg>`.
//
// It refuses to touch the string when the envelope is nested (another `<msg`
// inside the inner body) or when the leading text is not `<msg` at all
// (anti-injection: don't accidentally unwrap user-quoted content sitting in
// the middle of a sentence).
func StripOutputMsgEnvelope(s string) string {
	trimmed := strings.TrimSpace(s)
	if !strings.HasPrefix(trimmed, "<msg") {
		return s
	}

	// The opening must look like `<msg` followed by either `>` or whitespace.
	// `HasPrefix` only guarantees len >= 4, so the equality case (`<msg`
	// alone, no attributes, no closing) must be filtered out before indexing.
	if len(trimmed) <= len("<msg") {
		return s
	}
	switch trimmed[len("<msg")] {
	case '>', ' ', '\t':
	default:
		return s
	}

	_, body, ok := strings.Cut(trimmed, ">")
	if !ok {
		return s
	}

	// `</msg>` is missing when the API truncated the reply by max_tokens — in
	// that case we still drop the dangling opener and return the body as-is.
	inner, _, hasClose := strings.Cut(body, "</msg>")
	target := body
	if hasClose {
		target = inner
	}

	// Anti-injection: refuse to unwrap if a nested `<msg` token sits inside.
	if strings.Contains(target, "<msg") {
		return s
	}
	return strings.TrimSpace(target)
}

func isStripped(r rune) bool {
	if r <= 0x1F {
		return true
	}
	switch {
	case r >= '\u202A' && r <= '\u202E': // bidi overrides (LRE..RLO)
		return true
	case r >= '\u2066' && r <= '\u2069': // bidi isolates
		return true
	case r == '\u200B' || r == '\u200C' || r == '\u200D': // zero-width space/non-joiner/joiner
		return true
	case r == '\uFEFF': // BOM / zero-width no-break space
		return true
	}
	return false
}

func collapseSpaces(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if r == ' ' {
			if prevSpace {
				continue
			}
			prevSpace = true
			b.WriteRune(' ')
			continue
		}
		prevSpace = false
		b.WriteRune(r)
	}
	return b.String()
}

// TrimToSentences caps the model's output by two budgets at once:
// (1) at most maxSentences sentences, (2) at most maxRunes runes. A sentence
// boundary is a terminator from {. ! ? …}; consecutive terminators (!!!,
// ?!, !?) collapse into a single boundary. If the first maxSentences
// sentences together exceed maxRunes — we cut at the last sentence
// boundary that still fits the rune budget. If no boundary fits at all,
// the fallback is truncateRunes (no ellipsis added — we don't want to
// disguise a hard cut).
func TrimToSentences(s string, maxSentences int, maxRunes int) string {
	if maxSentences <= 0 || maxRunes <= 0 || s == "" {
		return ""
	}

	type boundary struct {
		byteEnd int // exclusive byte offset right after terminator run
		runes   int // total rune count up to byteEnd
	}

	var boundaries []boundary

	runeIdx := 0
	inTerminator := false
	for i, r := range s {
		runeIdx++
		if isSentenceTerminator(r) {
			inTerminator = true
			continue
		}
		if inTerminator {
			// previous run of terminators ended at byte index i (current rune start)
			boundaries = append(boundaries, boundary{
				byteEnd: i,
				runes:   runeIdx - 1,
			})
			inTerminator = false
		}
	}
	if inTerminator {
		// terminator run reaches end of string
		boundaries = append(boundaries, boundary{
			byteEnd: len(s),
			runes:   runeIdx,
		})
	}

	chosen := -1
	for i := 0; i < len(boundaries) && i < maxSentences; i++ {
		if boundaries[i].runes <= maxRunes {
			chosen = i
			continue
		}
		break
	}

	if chosen >= 0 {
		return s[:boundaries[chosen].byteEnd]
	}

	return truncateRunes(s, maxRunes)
}

func isSentenceTerminator(r rune) bool {
	switch r {
	case '.', '!', '?', '…':
		return true
	}
	return false
}

func truncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	count := 0
	for i := range s {
		if count == maxRunes {
			return s[:i]
		}
		count++
	}
	return s
}
