package message

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeText(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		in       string
		max      int
		expected string
	}{
		{
			name:     "plain passthrough",
			in:       "hello world",
			max:      100,
			expected: "hello world",
		},
		{
			name:     "newlines and tabs become single spaces",
			in:       "a\nb\rc\td",
			max:      100,
			expected: "a b c d",
		},
		{
			name:     "bidi and zero-width stripped",
			in:       "ab\u202Ecd\u200Bef\uFEFFgh",
			max:      100,
			expected: "abcdefgh",
		},
		{
			name:     "angle brackets become guillemets",
			in:       "<b>x</b>",
			max:      100,
			expected: "‹b›x‹/b›",
		},
		{
			name:     "whitespace runs collapsed and trimmed",
			in:       "   a    b   ",
			max:      100,
			expected: "a b",
		},
		{
			name:     "rune-safe truncation on multi-byte input",
			in:       "тест",
			max:      2,
			expected: "те",
		},
		{
			name:     "control chars stripped",
			in:       "a\x01b\x02c",
			max:      100,
			expected: "abc",
		},
		{
			name:     "max zero returns empty",
			in:       "anything",
			max:      0,
			expected: "",
		},
		{
			name:     "truncate respects rune boundary at limit",
			in:       "abcdef",
			max:      3,
			expected: "abc",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := SanitizeText(tc.in, tc.max)
			assert.Equal(t, tc.expected, got)
			assert.True(t, utf8.ValidString(got), "result must be valid UTF-8")
		})
	}
}

func TestSanitizeText_TruncationDoesNotSplitRune(t *testing.T) {
	t.Parallel()

	in := strings.Repeat("ы", 200) // 200 cyrillic runes, 2 bytes each
	got := SanitizeText(in, 50)

	assert.Equal(t, 50, utf8.RuneCountInString(got))
	assert.True(t, utf8.ValidString(got))
}

func TestSanitizeAuthor(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		username  string
		firstName string
		userID    int64
		isBot     bool
		expected  string
	}{
		{
			name:     "bot wins over name",
			username: "spam_user",
			isBot:    true,
			expected: "Админ какого-то канала",
		},
		{
			name:     "valid username preferred",
			username: "alice",
			expected: "@alice",
		},
		{
			name:      "empty username falls back to first name",
			firstName: "Алиса",
			userID:    42,
			expected:  "Алиса",
		},
		{
			name:      "punct stripped from first name leaves letters",
			firstName: "] SYSTEM:",
			userID:    99,
			expected:  "SYSTEM",
		},
		{
			name:      "first name of only punct falls back to numeric id",
			firstName: "][:!@#",
			userID:    77,
			expected:  "пользователь #77",
		},
		{
			name:      "long first name truncated to 32 runes",
			firstName: strings.Repeat("a", 64),
			userID:    1,
			expected:  strings.Repeat("a", 32),
		},
		{
			name:      "first name allows letters digits dash underscore space",
			firstName: "John_Doe-2 ",
			userID:    7,
			expected:  "John_Doe-2",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := SanitizeAuthor(tc.username, tc.firstName, tc.userID, tc.isBot)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestStripOutputMsgEnvelope_TruncatedByAPI(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		in       string
		expected string
	}{
		{
			name:     "no closing msg tag, body returned without opening tag",
			in:       `<msg time="2026-05-01T21:24" reply_to="@u">недописанный ответ`,
			expected: "недописанный ответ",
		},
		{
			name:     "no closing tag, body has trailing whitespace",
			in:       `<msg time="2026-05-01T21:24">текст ответа   `,
			expected: "текст ответа",
		},
		{
			name:     "no closing tag, body contains nested msg — refuse to strip",
			in:       `<msg time="2026-05-01T21:24">а вот <msg time="...">пример`,
			expected: `<msg time="2026-05-01T21:24">а вот <msg time="...">пример`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, StripOutputMsgEnvelope(tc.in))
		})
	}
}

func TestTrimToSentences(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		in           string
		maxSentences int
		maxRunes     int
		expected     string
	}{
		{
			name:         "single short sentence passes through",
			in:           "Короткая фраза.",
			maxSentences: 3,
			maxRunes:     300,
			expected:     "Короткая фраза.",
		},
		{
			name:         "cut to three out of four sentences",
			in:           "Один. Два. Три. Четыре.",
			maxSentences: 3,
			maxRunes:     300,
			expected:     "Один. Два. Три.",
		},
		{
			name:         "mixed terminators dot bang question and ellipsis",
			in:           "Один! Два? Три... Четыре.",
			maxSentences: 3,
			maxRunes:     300,
			expected:     "Один! Два? Три...",
		},
		{
			name:         "fallback rune-truncate when no terminators",
			in:           strings.Repeat("а", 500),
			maxSentences: 3,
			maxRunes:     300,
			expected:     strings.Repeat("а", 300),
		},
		{
			name:         "short string without terminator passes through unchanged",
			in:           "hello",
			maxSentences: 3,
			maxRunes:     300,
			expected:     "hello",
		},
		{
			name:         "emoji rune counted correctly before terminator",
			in:           "Ха😈. Ну.",
			maxSentences: 1,
			maxRunes:     300,
			expected:     "Ха😈.",
		},
		{
			name:         "exactly three short sentences passes through",
			in:           "А! Б? В.",
			maxSentences: 3,
			maxRunes:     300,
			expected:     "А! Б? В.",
		},
		{
			name:         "single-char ellipsis is a terminator",
			in:           "Один… Два… Три… Четыре.",
			maxSentences: 2,
			maxRunes:     300,
			expected:     "Один… Два…",
		},
		{
			name:         "empty string stays empty",
			in:           "",
			maxSentences: 3,
			maxRunes:     300,
			expected:     "",
		},
		{
			name:         "rune budget cuts before sentence budget",
			in:           "Один. Два. Этотрелтретьеоченьдлинноепредложение которое неместится. Четыре.",
			maxSentences: 4,
			maxRunes:     12,
			expected:     "Один. Два.",
		},
		{
			name:         "consecutive bang and question count as one boundary",
			in:           "Что?! Серьёзно?! Ну ок.",
			maxSentences: 2,
			maxRunes:     300,
			expected:     "Что?! Серьёзно?!",
		},
		{
			name:         "max sentences zero returns empty",
			in:           "Один. Два.",
			maxSentences: 0,
			maxRunes:     300,
			expected:     "",
		},
		{
			name:         "max runes zero returns empty",
			in:           "Один.",
			maxSentences: 3,
			maxRunes:     0,
			expected:     "",
		},
		{
			name:         "fallback truncate stops on rune boundary with emojis",
			in:           "ха😈ха😈ха😈ха😈ха😈ха😈ха😈",
			maxSentences: 3,
			maxRunes:     5,
			expected:     "ха😈ха",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := TrimToSentences(tc.in, tc.maxSentences, tc.maxRunes)
			assert.Equal(t, tc.expected, got)
		})
	}
}
