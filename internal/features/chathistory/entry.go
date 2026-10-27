package chathistory

import "time"

type Link struct {
	URL                string
	Anchor             string
	Title              string
	Description        string
	PreviewDescription string
}

type ReactionType struct {
	Type  string
	Value string
}

type Reaction struct {
	UpdateID       int
	MessageID      int
	ActorID        int64
	Actor          string
	Added          []ReactionType
	Removed        []ReactionType
	TargetKnown    bool
	TargetSenderID int64
	TargetAuthor   string
	TargetFromBot  bool
}

type Entry struct {
	ID        int
	SenderID  int64
	Time      time.Time
	Author    string
	Text      string
	Link      Link
	Reaction  *Reaction
	ReplyToID int
	FromBot   bool
	// PreFormatted indicates that Text already contains XML-like prompt markup
	// produced by the source handler (e.g., <photo>...</photo>). When true,
	// downstream prompt assembly must NOT re-sanitize the body, otherwise the
	// markup would be turned into guillemets and lose its semantics.
	PreFormatted bool
}

// ResolveTarget captures available authorship without copying the target's text.
// Previously captured authorship survives eviction of the target from history.
func (r Reaction) ResolveTarget(history []Entry) Reaction {
	for _, target := range history {
		if target.Reaction != nil || target.ID != r.MessageID {
			continue
		}

		r.TargetKnown = true
		r.TargetSenderID = target.SenderID
		r.TargetAuthor = target.Author
		r.TargetFromBot = target.FromBot
		break
	}

	return r
}
