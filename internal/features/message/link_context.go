package message

import (
	"strings"
	"unicode/utf8"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
)

const (
	maxLinkURLRunes                = 512
	maxLinkAnchorRunes             = 200
	maxLinkTitleRunes              = 200
	maxLinkDescriptionRunes        = 500
	maxLinkPreviewDescriptionRunes = 500
	maxMeaningfulEntryBytes        = 1000
)

func writeEntryBody(b *strings.Builder, entry chathistory.Entry) {
	if entry.PreFormatted {
		b.WriteString(entry.Text)
	} else {
		b.WriteString(SanitizeText(entry.Text, maxEntryRunes))
	}
	link := sanitizeLink(entry.Link)
	if !hasLinkContext(link) {
		return
	}

	fields := []struct {
		tag   string
		value string
	}{
		{tag: "url", value: link.URL},
		{tag: "anchor", value: link.Anchor},
		{tag: "title", value: link.Title},
		{tag: "description", value: link.Description},
		{tag: "preview_description", value: link.PreviewDescription},
	}
	b.WriteString("\n<link>")
	for _, field := range fields {
		if field.value == "" {
			continue
		}
		b.WriteByte('<')
		b.WriteString(field.tag)
		b.WriteByte('>')
		b.WriteString(field.value)
		b.WriteString("</")
		b.WriteString(field.tag)
		b.WriteByte('>')
	}
	b.WriteString("</link>")
}

func meaningfulEntryText(entry chathistory.Entry) string {
	link := sanitizeLink(entry.Link)
	if !hasLinkContext(link) {
		return entry.Text
	}

	fields := []string{
		link.Title,
		link.Description,
		link.PreviewDescription,
		link.Anchor,
		SanitizeText(entry.Text, maxMeaningfulEntryBytes),
	}
	var b strings.Builder
	for _, field := range fields {
		if field == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(field)
	}
	text := b.String()
	if len(text) <= maxMeaningfulEntryBytes {
		return text
	}
	end := maxMeaningfulEntryBytes
	for !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end]
}

func sanitizeLink(link chathistory.Link) chathistory.Link {
	return chathistory.Link{
		URL:                SanitizeText(link.URL, maxLinkURLRunes),
		Anchor:             SanitizeText(link.Anchor, maxLinkAnchorRunes),
		Title:              SanitizeText(link.Title, maxLinkTitleRunes),
		Description:        SanitizeText(link.Description, maxLinkDescriptionRunes),
		PreviewDescription: SanitizeText(link.PreviewDescription, maxLinkPreviewDescriptionRunes),
	}
}

func hasLinkContext(link chathistory.Link) bool {
	return link.Title != "" || link.Description != "" || link.PreviewDescription != ""
}
