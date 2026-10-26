package linkextract

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/telebot.v3"
)

// spec: LINK-001
// spec: LINK-002
// spec: LINK-003
// spec: LINK-017
func TestFirst(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		text     string
		entities telebot.Entities
		want     Reference
	}{
		{name: "plain", text: "смотри https://example.com/a?q=1#part и https://second.example/", want: Reference{URL: "https://example.com/a?q=1#part"}},
		{name: "emoji_hidden", text: "😀 почитай тут", entities: telebot.Entities{{Type: telebot.EntityTextLink, Offset: 3, Length: 11, URL: "https://example.com/"}}, want: Reference{URL: "https://example.com/", Anchor: "почитай тут"}},
		{name: "visible_entity", text: "😀 https://example.com/", entities: telebot.Entities{{Type: telebot.EntityURL, Offset: 3, Length: 20}}, want: Reference{URL: "https://example.com/"}},
		{name: "schemeless_first_entity", text: "habr.com/a https://second.example/", entities: telebot.Entities{{Type: telebot.EntityURL, Offset: 11, Length: 23}, {Type: telebot.EntityURL, Offset: 0, Length: 10}}, want: Reference{URL: "http://habr.com/a"}},
		{name: "schemeless_private_first_entity", text: "127.0.0.1/a https://second.example/", entities: telebot.Entities{{Type: telebot.EntityURL, Offset: 0, Length: 11}, {Type: telebot.EntityURL, Offset: 12, Length: 23}}, want: Reference{URL: "http://127.0.0.1/a"}},
		{name: "unsupported_visible_entity", text: "ftp://example.com/ https://second.example/", entities: telebot.Entities{{Type: telebot.EntityURL, Offset: 0, Length: 18}, {Type: telebot.EntityURL, Offset: 19, Length: 23}}, want: Reference{URL: "https://second.example/"}},
		{name: "order_by_position", text: "one two", entities: telebot.Entities{{Type: telebot.EntityTextLink, Offset: 4, Length: 3, URL: "https://second.example/"}, {Type: telebot.EntityTextLink, Offset: 0, Length: 3, URL: "https://first.example/"}}, want: Reference{URL: "https://first.example/", Anchor: "one"}},
		{name: "unsupported_then_supported", text: "one two", entities: telebot.Entities{{Type: telebot.EntityTextLink, Offset: 0, Length: 3, URL: "tg://user?id=1"}, {Type: telebot.EntityTextLink, Offset: 4, Length: 3, URL: "http://example.com/"}}, want: Reference{URL: "http://example.com/", Anchor: "two"}},
		{name: "split_surrogate", text: "😀 two", entities: telebot.Entities{{Type: telebot.EntityTextLink, Offset: 1, Length: 1, URL: "https://bad.example/"}, {Type: telebot.EntityTextLink, Offset: 3, Length: 3, URL: "https://example.com/"}}, want: Reference{URL: "https://example.com/", Anchor: "two"}},
		{name: "split_end_surrogate", text: "x😀", entities: telebot.Entities{{Type: telebot.EntityTextLink, Offset: 0, Length: 2, URL: "https://bad.example/"}}},
		{name: "negative_bounds", text: "x", entities: telebot.Entities{{Type: telebot.EntityTextLink, Offset: -1, Length: 1, URL: "https://example.com/"}}},
		{name: "outside_bounds", text: "x", entities: telebot.Entities{{Type: telebot.EntityURL, Offset: 0, Length: 99}}},
		{name: "zero_length", text: "x", entities: telebot.Entities{{Type: telebot.EntityTextLink, Offset: 0, URL: "https://example.com/"}}},
		{name: "no_url", text: "example.com и ftp://example.com"},
		{name: "punctuation", text: "(https://example.com/a_(b)).", want: Reference{URL: "https://example.com/a_(b)"}},
		{name: "first_forbidden_not_replaced", text: "http://127.0.0.1/ https://example.com/", want: Reference{URL: "http://127.0.0.1/"}},
		{name: "userinfo_not_replaced", text: "https://user@example.com/ https://second.example/", want: Reference{URL: "https://user@example.com/"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, First(tc.text, tc.entities))
		})
	}
}

// spec: LINK-001
// spec: LINK-018
func TestCaptionIgnoresTelegramPreviewOptions(t *testing.T) {
	t.Parallel()
	for _, options := range []string{`null`, `{"is_disabled":true}`, `{"is_disabled":false,"url":"https://other.example/"}`} {
		var msg telebot.Message
		require.NoError(
			t,
			json.Unmarshal(
				[]byte(
					`{"caption":"😀 тут","caption_entities":[{"type":"text_link","offset":3,"length":3,"url":"https://example.com/"}],"link_preview_options":`+options+`}`,
				),
				&msg,
			),
		)
		assert.Equal(
			t,
			Reference{URL: "https://example.com/", Anchor: "тут"},
			First(msg.Caption, msg.CaptionEntities),
		)
	}
}
