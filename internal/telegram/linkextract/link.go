package linkextract

import (
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"gopkg.in/telebot.v3"
)

var explicitURL = regexp.MustCompile(`(?i)https?://[^\s<>"\x00-\x1f]+`)

type Reference struct {
	URL    string
	Anchor string
}

// First selects before network validation: a forbidden first address must never
// cause a later link to be loaded instead.
func First(text string, entities telebot.Entities) Reference {
	if len(entities) == 0 {
		for _, raw := range explicitURL.FindAllString(text, -1) {
			raw = trimPunctuation(raw)
			if isHTTP(raw) {
				return Reference{URL: raw}
			}
		}
		return Reference{}
	}

	var first Reference
	firstOffset := len(text) + 1
	for _, entity := range entities {
		if entity.Type != telebot.EntityURL && entity.Type != telebot.EntityTextLink {
			continue
		}
		visible, ok := utf16Range(text, entity.Offset, entity.Length)
		if !ok || entity.Offset >= firstOffset {
			continue
		}
		ref := Reference{URL: visible}
		if entity.Type == telebot.EntityTextLink {
			ref = Reference{URL: entity.URL, Anchor: visible}
		} else if parsed, err := url.Parse(visible); err == nil && parsed.Scheme == "" {
			ref.URL = "http://" + visible
		}
		if !isHTTP(ref.URL) {
			continue
		}
		first, firstOffset = ref, entity.Offset
	}
	return first
}

func isHTTP(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil &&
		(strings.EqualFold(u.Scheme, "http") || strings.EqualFold(u.Scheme, "https")) &&
		u.Hostname() != ""
}

func utf16Range(text string, offset, length int) (string, bool) {
	if offset < 0 || length <= 0 || offset > len(text) || length > len(text)-offset {
		return "", false
	}
	end := offset + length
	startByte, endByte := -1, -1
	units := 0
	for pos, r := range text {
		if units == offset {
			startByte = pos
		}
		if units == end {
			endByte = pos
			break
		}
		units++
		if r > 0xffff {
			units++
		}
	}
	if units == end && endByte < 0 {
		endByte = len(text)
	}
	if startByte < 0 || endByte < 0 || !utf8.ValidString(text[startByte:endByte]) {
		return "", false
	}
	return text[startByte:endByte], true
}

func trimPunctuation(raw string) string {
	raw = strings.TrimRight(raw, ".,!;")
	for _, pair := range [][2]string{{"(", ")"}, {"[", "]"}, {"{", "}"}} {
		for strings.HasSuffix(raw, pair[1]) && strings.Count(raw, pair[1]) > strings.Count(raw, pair[0]) {
			raw = strings.TrimSuffix(raw, pair[1])
		}
	}
	return raw
}
