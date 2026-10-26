package linkpreview

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"net/url"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

const (
	maxHTMLBytes        = 1024 * 1024
	maxCharsetBytes     = 1024
	maxTitleRunes       = 200
	maxDescriptionRunes = 500
)

type headParser struct {
	metadata                      Metadata
	imageRef, baseRef, rawElement string
	templateDepth                 int
	started                       bool
}

func parseHead(body io.Reader, contentType string, target *url.URL) (Metadata, error) {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "text/html" {
		return Metadata{}, errors.New("content_type")
	}
	bounded := &io.LimitedReader{R: body, N: maxHTMLBytes + 1}
	decoded, sniffErr := decodedHeadReader(bounded, contentType)
	tokenizer := html.NewTokenizer(decoded)
	tokenizer.SetMaxBuf(maxHTMLBytes)
	var parser headParser
	var parseErr error
	if sniffErr != nil {
		parseErr = errors.New("html_read")
	}
	for {
		kind := tokenizer.Next()
		if bounded.N <= 0 {
			parseErr = errors.New("html_limit")
			break
		}
		if kind == html.ErrorToken {
			if !errors.Is(tokenizer.Err(), io.EOF) {
				parseErr = errors.New("html_read")
			}
			break
		}
		if parser.consume(kind, tokenizer.Token()) {
			break
		}
	}
	base := target
	if candidate, err := resolvedURL(target, parser.baseRef); parser.baseRef != "" && err == nil {
		base = candidate
	}
	if candidate, err := resolvedURL(base, parser.imageRef); parser.imageRef != "" && err == nil {
		parser.metadata.ImageURL = candidate.String()
	}
	return parser.metadata, parseErr
}

func decodedHeadReader(body io.Reader, contentType string) (io.Reader, error) {
	var prefix bytes.Buffer
	tokenizer := html.NewTokenizer(io.TeeReader(io.LimitReader(body, maxCharsetBytes), &prefix))
	var parser headParser
	var readErr error
	for {
		kind := tokenizer.Next()
		if kind == html.ErrorToken {
			if !errors.Is(tokenizer.Err(), io.EOF) {
				readErr = tokenizer.Err()
			}
			break
		}
		if parser.consume(kind, tokenizer.Token()) {
			break
		}
	}
	// Charset sniffing must not wait to fill 1024 bytes after an already complete head.
	encoding, _, _ := charset.DetermineEncoding(prefix.Bytes(), contentType)
	var replay io.Reader = bytes.NewReader(prefix.Bytes())
	if readErr == nil {
		replay = io.MultiReader(replay, body)
	}
	return encoding.NewDecoder().Reader(replay), readErr
}

func (p *headParser) consume(kind html.TokenType, token html.Token) bool {
	if !p.started {
		token.Data = strings.TrimPrefix(token.Data, "\ufeff")
		p.started = true
	}
	switch kind {
	case html.EndTagToken:
		if token.Data == "head" || token.Data == "html" {
			return true
		}
		if token.Data == "template" && p.templateDepth > 0 {
			p.templateDepth--
		}
		if token.Data == p.rawElement {
			p.rawElement = ""
		}
	case html.TextToken:
		return p.rawElement == "" && p.templateDepth == 0 && strings.TrimSpace(token.Data) != ""
	case html.StartTagToken, html.SelfClosingTagToken:
		return p.startTag(token)
	case html.ErrorToken, html.CommentToken, html.DoctypeToken:
	}
	return false
}

func (p *headParser) startTag(token html.Token) bool {
	if token.Data == "template" {
		p.templateDepth++
		return false
	}
	if p.templateDepth > 0 {
		return false
	}
	switch token.Data {
	case "html", "head", "basefont", "bgsound", "link":
	case "script", "style", "title", "noscript", "noframes":
		p.rawElement = token.Data
	case "base":
		if p.baseRef == "" {
			p.baseRef = firstAttribute(token, "href")
		}
	case "meta":
		p.meta(token)
	default:
		return true
	}
	return false
}

func (p *headParser) meta(token html.Token) {
	property := firstAttribute(token, "property")
	if property == "" {
		property = firstAttribute(token, "name")
	}
	value := strings.TrimSpace(firstAttribute(token, "content"))
	if value == "" {
		return
	}
	switch strings.ToLower(property) {
	case "og:title":
		if p.metadata.Title == "" {
			p.metadata.Title = limitRunes(value, maxTitleRunes)
		}
	case "og:description":
		if p.metadata.Description == "" {
			p.metadata.Description = limitRunes(value, maxDescriptionRunes)
		}
	case "og:image":
		if p.imageRef == "" {
			p.imageRef = value
		}
	}
}

func firstAttribute(token html.Token, name string) string {
	for _, attr := range token.Attr {
		if attr.Key == name {
			return attr.Val
		}
	}
	return ""
}

func resolvedURL(base *url.URL, raw string) (*url.URL, error) {
	relative, err := url.Parse(raw)
	if err != nil {
		return nil, errors.New("invalid_url")
	}
	return ParseURL(base.ResolveReference(relative).String())
}

func limitRunes(text string, maxRunes int) string {
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes])
}
