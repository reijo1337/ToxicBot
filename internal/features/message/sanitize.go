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
