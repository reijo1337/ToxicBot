package cases

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTagAttributionAIHasKnownTargetHistory(t *testing.T) {
	t.Parallel()
	cs, err := Load(filepath.Join("..", "..", "testdata", "tag-attribution.json"))
	require.NoError(t, err)
	require.Len(t, cs, 6)
	for _, c := range cs {
		t.Run(c.ID, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, KindTag, c.Kind)
			require.NotNil(t, c.Target)
			require.Positive(t, c.Target.UserID)
			var own, botEntries int
			for _, e := range c.DomainHistory() {
				if e.FromBot {
					botEntries++
					continue
				}
				if e.SenderID == c.Target.UserID && strings.TrimSpace(e.Text) != "" {
					own++
				}
			}
			assert.Positive(t, own, "AI fixture needs a nonempty known message from the target")
			assert.Positive(t, botEntries, "keep prior bot statements as exclusion inputs")
		})
	}
}

func TestTagAttributionFixtureIdentityRelations(t *testing.T) {
	t.Parallel()
	tests := []struct {
		file       string
		id         string
		targetID   int64
		ownIDs     []int
		foreignIDs []int
		legacyIDs  []int
		botIDs     []int
	}{
		{
			file: "tag-attribution.json", id: "tag-phone-discussion-author", targetID: 91002,
			ownIDs: []int{1, 2}, foreignIDs: []int{3}, botIDs: []int{4},
		},
		{
			file: "tag-attribution.json", id: "tag-own-before-foreign-threat", targetID: 91001,
			ownIDs: []int{1}, foreignIDs: []int{3}, botIDs: []int{2},
		},
		{
			file: "tag-attribution.json", id: "tag-identical-authors", targetID: 91001,
			ownIDs: []int{1}, foreignIDs: []int{2}, botIDs: []int{3},
		},
		{
			file: "tag-attribution.json", id: "tag-renamed-account", targetID: 91001,
			ownIDs: []int{1, 3}, foreignIDs: []int{2}, botIDs: []int{4},
		},
		{
			file: "tag-attribution.json", id: "tag-legacy-with-known-author", targetID: 91001,
			ownIDs: []int{5}, foreignIDs: []int{3}, legacyIDs: []int{1, 2}, botIDs: []int{4},
		},
		{
			file: "tag-attribution.json", id: "tag-own-question-about-other-phone", targetID: 91001,
			ownIDs: []int{2}, foreignIDs: []int{1, 3}, botIDs: []int{4},
		},
		{
			file: "tag-attribution-negative.json", id: "tag-absent-phone", targetID: 91001,
			foreignIDs: []int{1, 2, 3}, botIDs: []int{4},
		},
		{
			file: "tag-attribution-negative.json", id: "tag-legacy-unknown-sender", targetID: 91001,
			foreignIDs: []int{3}, legacyIDs: []int{1, 2}, botIDs: []int{4},
		},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			t.Parallel()
			cs, err := Load(filepath.Join("..", "..", "testdata", tt.file))
			require.NoError(t, err)
			var fixture *Case
			for i := range cs {
				if cs[i].ID == tt.id {
					fixture = &cs[i]
					break
				}
			}
			require.NotNil(t, fixture)
			require.NotNil(t, fixture.Target)
			assert.Equal(t, tt.targetID, fixture.Target.UserID)
			var ownIDs, foreignIDs, legacyIDs, botIDs []int
			for _, e := range fixture.DomainHistory() {
				require.NotEmpty(t, strings.TrimSpace(e.Text))
				switch {
				case e.FromBot:
					botIDs = append(botIDs, e.ID)
				case e.SenderID == tt.targetID:
					ownIDs = append(ownIDs, e.ID)
				case e.SenderID > 0:
					foreignIDs = append(foreignIDs, e.ID)
				default:
					legacyIDs = append(legacyIDs, e.ID)
				}
			}
			assert.Equal(t, tt.ownIDs, ownIDs)
			assert.Equal(t, tt.foreignIDs, foreignIDs)
			assert.Equal(t, tt.legacyIDs, legacyIDs)
			assert.Equal(t, tt.botIDs, botIDs)
		})
	}
}
