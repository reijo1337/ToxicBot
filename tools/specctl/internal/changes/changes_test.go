package changes

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	histCap  = "chat-history"
	photoCap = "photo-reactions"
)

func mkChange(t *testing.T, root, id string, deltas ...string) {
	t.Helper()

	dir := filepath.Join(root, id)
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "proposal.md"), []byte("# "+id), 0o600))

	for _, d := range deltas {
		deltaDir := filepath.Join(dir, "specs", d)
		require.NoError(t, os.MkdirAll(deltaDir, 0o750))
		require.NoError(t, os.WriteFile(
			filepath.Join(deltaDir, "spec.md"), []byte("## ADDED Requirements"), 0o600,
		))
	}
}

func TestActive_CollectsChangesAndDeltas(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mkChange(t, root, "fix-photo-loop", photoCap, histCap)
	mkChange(t, root, "no-delta-change")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "archive", "2026-01-01-old"), 0o750))

	got, err := Active(root)
	require.NoError(t, err)
	require.Len(t, got, 2)

	assert.Equal(t, "fix-photo-loop", got[0].ID)
	assert.Equal(t, []string{histCap, photoCap}, got[0].Capabilities)
	assert.Equal(t, "no-delta-change", got[1].ID)
	assert.Empty(t, got[1].Capabilities)
}

func TestActive_MissingDirIsEmpty(t *testing.T) {
	t.Parallel()

	got, err := Active(filepath.Join(t.TempDir(), "нет-такого"))
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestActive_CollectsDeltaScenarios(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mkChange(t, root, "fix-photo-loop", photoCap)
	delta := "## ADDED Requirements\n\n### Requirement: Подпись к фото\n\n" +
		"#### Scenario: [PHOTO-012] подпись есть\n- THEN x\n\n" +
		"## MODIFIED Requirements\n\n### Requirement: Реакция\n\n" +
		"#### Scenario: [PHOTO-003] своё фото\n- THEN y\n"
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "fix-photo-loop", "specs", photoCap, "spec.md"), []byte(delta), 0o600,
	))

	got, err := Active(root)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, []string{"PHOTO-003", "PHOTO-012"}, got[0].Scenarios)
}

func TestActive_DeltaScenarioWithoutIDFails(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mkChange(t, root, "bad", photoCap)
	delta := "## ADDED Requirements\n\n### Requirement: Р\n\n#### Scenario: без номера\n"
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "bad", "specs", photoCap, "spec.md"), []byte(delta), 0o600,
	))

	_, err := Active(root)
	require.Error(t, err)
}

func TestPendingScenarios(t *testing.T) {
	t.Parallel()

	got := PendingScenarios([]Change{
		{ID: "a", Scenarios: []string{"PHOTO-012"}},
		{ID: "b", Scenarios: []string{"HIST-020", "PHOTO-012"}},
	})
	assert.Equal(t, map[string]struct{}{"PHOTO-012": {}, "HIST-020": {}}, got)
}

func TestTouchedCapabilities(t *testing.T) {
	t.Parallel()

	list := []Change{
		{ID: "a", Capabilities: []string{photoCap}},
		{ID: "b", Capabilities: []string{histCap, photoCap}},
	}

	got := TouchedCapabilities(list)
	assert.Equal(t, map[string]string{
		photoCap: "a",
		histCap:  "b",
	}, got)
}
