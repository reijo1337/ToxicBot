package check

import (
	"testing"

	"github.com/reijo1337/ToxicBot/tools/specctl/internal/anchors"
	"github.com/reijo1337/ToxicBot/tools/specctl/internal/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	photoPkg   = "internal/handlers/on_photo"
	photoOne   = "PHOTO-001"
	cmdPkg     = "cmd"
	photoCapNm = "on_photo"
)

// photoCapability собирает capability фото-реакций с заданными статусами сценариев.
func photoCapability(scen map[string]spec.Coverage) spec.Capability {
	specs := make([]spec.Scenario, 0, len(scen))
	for id := range scen {
		specs = append(specs, spec.Scenario{ID: id, Title: id})
	}

	return spec.Capability{
		Name:      photoCapNm,
		Prefix:    "PHOTO",
		Owns:      []string{photoPkg},
		Scenarios: scen,
		Specs:     specs,
	}
}

func TestVerify_CoveredWithoutAnchor(t *testing.T) {
	t.Parallel()

	caps := []spec.Capability{
		photoCapability(map[string]spec.Coverage{photoOne: spec.CoverageCovered}),
	}

	got := Verify(caps, nil)
	require.Len(t, got, 1)
	assert.Equal(t, SeverityFail, got[0].Severity)
	assert.Contains(t, got[0].Message, photoOne)
	assert.Contains(t, got[0].Message, "нет теста с якорем")
}

func TestVerify_CoveredWithAnchor(t *testing.T) {
	t.Parallel()

	caps := []spec.Capability{
		photoCapability(map[string]spec.Coverage{photoOne: spec.CoverageCovered}),
	}
	found := []anchors.Anchor{{ScenarioID: photoOne, TestName: "TestX", File: "x_test.go"}}

	assert.Empty(t, Verify(caps, found))
}

func TestVerify_AnchorToUnknownScenario(t *testing.T) {
	t.Parallel()

	caps := []spec.Capability{
		photoCapability(map[string]spec.Coverage{photoOne: spec.CoverageCovered}),
	}
	found := []anchors.Anchor{
		{ScenarioID: photoOne, TestName: "TestX", File: "x_test.go"},
		{ScenarioID: "PHOTO-404", TestName: "TestGhost", File: "g_test.go", Line: 12},
	}

	got := Verify(caps, found)
	require.Len(t, got, 1)
	assert.Equal(t, SeverityFail, got[0].Severity)
	assert.Contains(t, got[0].Message, "PHOTO-404")
	assert.Contains(t, got[0].Message, "не существует")
	assert.Equal(t, "g_test.go:12", got[0].Location)
}

func TestVerify_ManualAndTodoNeedNoAnchor(t *testing.T) {
	t.Parallel()

	caps := []spec.Capability{
		photoCapability(map[string]spec.Coverage{
			photoOne:    spec.CoverageManual,
			"PHOTO-002": spec.CoverageTodo,
		}),
	}

	assert.Empty(t, Verify(caps, nil))
}

func TestOrphan(t *testing.T) {
	t.Parallel()

	caps := []spec.Capability{
		photoCapability(nil),
	}
	packages := []string{
		photoPkg,
		"internal/handlers/on_voice",
		cmdPkg,
	}

	got := Orphan(caps, packages, []string{cmdPkg})
	require.Len(t, got, 1)
	assert.Equal(t, SeverityWarn, got[0].Severity)
	assert.Equal(t, "internal/handlers/on_voice", got[0].Location)
}

func TestOrphan_NestedPackageBelongsToOwner(t *testing.T) {
	t.Parallel()

	caps := []spec.Capability{
		photoCapability(nil),
	}
	packages := []string{"internal/handlers/on_photo/inner"}

	assert.Empty(t, Orphan(caps, packages, nil))
}

func TestHasFailures(t *testing.T) {
	t.Parallel()

	assert.False(t, HasFailures(nil))
	assert.False(t, HasFailures([]Finding{{Severity: SeverityWarn}}))
	assert.True(t, HasFailures([]Finding{{Severity: SeverityWarn}, {Severity: SeverityFail}}))
}

func TestOrphan_FileLevelOwnershipCoversPackage(t *testing.T) {
	t.Parallel()

	caps := []spec.Capability{{
		Name: "chat-history",
		Owns: []string{"internal/infrastructure/storage/db/chat_history.go"},
	}}
	packages := []string{"internal/infrastructure/storage/db"}

	assert.Empty(t, Orphan(caps, packages, nil))
}

func TestCovers(t *testing.T) {
	t.Parallel()

	owned := []string{photoPkg, "internal/storage/db/chat_history.go"}

	assert.True(t, Covers(owned, photoPkg))
	assert.True(t, Covers(owned, "internal/handlers/on_photo/deep/file.go"))
	assert.True(t, Covers(owned, "internal/storage/db/chat_history.go"))
	assert.False(t, Covers(owned, "internal/storage/db/chat_settings.go"))
	assert.False(t, Covers(owned, "internal/handlers/on_photo_extra"))
}
