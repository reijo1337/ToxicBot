package message

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStripOutputMsgEnvelope(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		in       string
		expected string
	}{
		{
			name:     "plain text without envelope",
			in:       "плохой ответ",
			expected: "плохой ответ",
		},
		{
			name:     "envelope with only time attribute",
			in:       `<msg time="2026-05-01T21:24">плохой</msg>`,
			expected: "плохой",
		},
		{
			name:     "envelope with time and reply_to attributes",
			in:       `<msg time="2026-05-01T21:24" reply_to="@x">плохой</msg>`,
			expected: "плохой",
		},
		{
			name:     "envelope wrapped in whitespace",
			in:       "  <msg>X</msg>  ",
			expected: "X",
		},
		{
			name:     "trailing chatter after closing tag is stripped",
			in:       "<msg>A</msg> bare tail",
			expected: "A",
		},
		{
			name:     "trailing chatter with attributes is stripped",
			in:       `<msg time="2026-05-01T21:24">плохой</msg>  и ещё немного`,
			expected: "плохой",
		},
		{
			name:     "lone opening tag without body is left alone",
			in:       "<msg",
			expected: "<msg",
		},
		{
			name:     "head before envelope not stripped",
			in:       "head <msg>X</msg>",
			expected: "head <msg>X</msg>",
		},
		{
			name:     "nested envelope left alone",
			in:       "<msg>nested <msg>x</msg> outer</msg>",
			expected: "<msg>nested <msg>x</msg> outer</msg>",
		},
		{
			name:     "empty string",
			in:       "",
			expected: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := StripOutputMsgEnvelope(tc.in)
			assert.Equal(t, tc.expected, got)
		})
	}
}
