package chathistory

import "time"

type Entry struct {
	ID        int
	Time      time.Time
	Author    string
	Text      string
	ReplyToID int
	FromBot   bool
	// PreFormatted indicates that Text already contains XML-like prompt markup
	// produced by the source handler (e.g., <photo>...</photo>). When true,
	// downstream prompt assembly must NOT re-sanitize the body, otherwise the
	// markup would be turned into guillemets and lose its semantics.
	PreFormatted bool
}
