package chathistory_test

import (
	"testing"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/stretchr/testify/assert"
)

// spec: HIST-025
func TestBuffer_LinkSnapshotsAreValues(t *testing.T) {
	t.Parallel()

	buf := chathistory.NewBuffer(10)
	entry := chathistory.Entry{
		ID: 1, SenderID: 42, Author: "alice", Text: "почитай тут",
		Link: chathistory.Link{
			URL: "https://example.com", Anchor: "почитай тут", Title: "title",
			Description: "description", PreviewDescription: "preview",
		},
	}
	want := entry
	buf.Add(1, entry)
	entry.Link.Title = "changed input"
	snapshot := buf.Get(1)
	snapshot[0].Link.Description = "changed snapshot"

	assert.Equal(t, []chathistory.Entry{want}, buf.Get(1))
}

// spec: HIST-025
func TestBuffer_AddAllLinkSnapshotsAreValues(t *testing.T) {
	t.Parallel()

	buf := chathistory.NewBuffer(10)
	entries := []chathistory.Entry{
		{ID: 1, Link: chathistory.Link{URL: "https://example.com", Anchor: "тут"}},
	}
	want := entries[0]
	buf.AddAll(1, entries...)
	entries[0].Link.Anchor = "changed input"
	got := buf.Get(1)
	got[0].Link.URL = "https://changed.example"

	assert.Equal(t, []chathistory.Entry{want}, buf.Get(1))
}
