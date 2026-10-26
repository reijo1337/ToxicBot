package chathistory

import "time"

type Link struct {
	URL                string
	Anchor             string
	Title              string
	Description        string
	PreviewDescription string
}

type Entry struct {
	ID        int
	SenderID  int64
	Time      time.Time
	Author    string
	Text      string
	Link      Link
	ReplyToID int
	FromBot   bool
	// PreFormatted indicates that Text already contains XML-like prompt markup
	// produced by the source handler (e.g., <photo>...</photo>). When true,
	// downstream prompt assembly must NOT re-sanitize the body, otherwise the
	// markup would be turned into guillemets and lose its semantics.
	PreFormatted bool
}
