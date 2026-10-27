package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/reijo1337/ToxicBot/tools/specctl/internal/changes"
	"github.com/reijo1337/ToxicBot/tools/specctl/internal/check"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	photoMetadata = "prefix: PHOTO\nowns: [internal/handlers/on_photo]\nscenarios:\n  PHOTO-001: covered\n"
	photoLiving   = "### Requirement: Photos\n#### Scenario: [PHOTO-001] old\n"
	photoDelta    = "## ADDED Requirements\n### Requirement: Photos\n#### Scenario: [PHOTO-002] new\n"
	photoAnchor   = "package sample\nimport \"testing\"\n// spec: PHOTO-001\nfunc TestOld(t *testing.T) {}\n"
)

func TestVerify_ActiveDeltasJoinLivingScenariosOfSameCapability(t *testing.T) {
	t.Parallel()

	root := readyRoot(t, nil, nil)
	writeSpecctlFile(
		t,
		root,
		"openspec/specs/photo-reactions/capability.yaml",
		photoMetadata+"  PHOTO-002: covered\n  PHOTO-003: manual\nnotes:\n  PHOTO-003: needs live Telegram\n",
	)
	writeSpecctlFile(t, root, "openspec/specs/photo-reactions/spec.md", photoLiving)
	writeSpecctlFile(t, root, "openspec/changes/first/specs/photo-reactions/spec.md", photoDelta)
	writeSpecctlFile(t, root, "openspec/changes/second/specs/photo-reactions/spec.md",
		"## MODIFIED Requirements\n### Requirement: Photos\n#### Scenario: [PHOTO-001] revised\n"+
			"#### Scenario: [PHOTO-003] manual check\n")
	writeSpecctlFile(t, root, "internal/handlers/on_photo/photo_test.go", photoAnchor+
		"// spec: PHOTO-002\nfunc TestNew(t *testing.T) {}\n")

	require.NoError(t, run([]string{"verify", "--root", root}))

	loaded, err := load(root)

	require.NoError(t, err)
	require.Len(t, loaded.caps, 1)
	assert.Len(t, loaded.caps[0].Specs, 3, "a modified scenario must appear only once in the union")
	//nolint:gosec // fixture under t.TempDir
	body, err := os.ReadFile(filepath.Join(root, "openspec/specs/photo-reactions/spec.md"))

	require.NoError(t, err)
	assert.Equal(t, photoLiving, string(body), "loading must leave living specs untouched")
}

func TestVerify_NewCapabilityUsesItsActiveDeltaUntilArchive(t *testing.T) {
	t.Parallel()

	root := readyRoot(t, nil, nil)
	writeSpecctlFile(t, root, "openspec/specs/photo-reactions/capability.yaml", photoMetadata)
	writeSpecctlFile(
		t,
		root,
		"openspec/changes/new-photos/specs/photo-reactions/spec.md",
		photoLiving,
	)
	writeSpecctlFile(t, root, "internal/handlers/on_photo/photo_test.go", photoAnchor)

	require.NoError(t, run([]string{"verify", "--root", root}))

	_, err := os.Stat(filepath.Join(root, "openspec/specs/photo-reactions/spec.md"))

	require.ErrorIs(t, err, os.ErrNotExist, "a new living spec must not be created before archive")
}

func TestVerify_ActiveDeltaCoveredScenarioStillRequiresAnchor(t *testing.T) {
	t.Parallel()

	root := readyRoot(t, nil, nil)
	writeSpecctlFile(
		t,
		root,
		"openspec/specs/photo-reactions/capability.yaml",
		photoMetadata+"  PHOTO-002: covered\n",
	)
	writeSpecctlFile(t, root, "openspec/specs/photo-reactions/spec.md", photoLiving)
	writeSpecctlFile(
		t,
		root,
		"openspec/changes/new-photos/specs/photo-reactions/spec.md",
		photoDelta,
	)
	writeSpecctlFile(t, root, "internal/handlers/on_photo/photo_test.go", photoAnchor)

	loaded, err := load(root)

	require.NoError(t, err, "loading must reach the existing anchor check")

	findings := check.Verify(loaded.caps, loaded.anchors, changes.PendingScenarios(loaded.active))

	require.Len(t, findings, 1)
	assert.Contains(t, findings[0].Message, "PHOTO-002 помечен covered, но нет теста")
	require.Error(t, run([]string{"verify", "--root", root}))
}

func TestLoad_ActiveDeltasDoNotRelaxMetadataOrFileValidation(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		files   map[string]string
		wantErr string
	}{
		"foreign capability cannot authorize an ID": {
			files: map[string]string{
				"openspec/specs/photo-reactions/capability.yaml":          photoMetadata + "  PHOTO-002: todo\n",
				"openspec/specs/photo-reactions/spec.md":                  photoLiving,
				"openspec/changes/new/specs/different-capability/spec.md": photoDelta,
			},
			wantErr: "PHOTO-002 указан в capability.yaml, но отсутствует",
		},
		"archived delta cannot authorize an ID": {
			files: map[string]string{
				"openspec/specs/photo-reactions/capability.yaml":             photoMetadata + "  PHOTO-002: todo\n",
				"openspec/specs/photo-reactions/spec.md":                     photoLiving,
				"openspec/changes/archive/old/specs/photo-reactions/spec.md": photoDelta,
			},
			wantErr: "PHOTO-002 указан в capability.yaml, но отсутствует",
		},
		"unknown status": {
			files: map[string]string{
				"openspec/specs/photo-reactions/capability.yaml":     photoMetadata + "  PHOTO-002: maybe\n",
				"openspec/specs/photo-reactions/spec.md":             photoLiving,
				"openspec/changes/new/specs/photo-reactions/spec.md": photoDelta,
			},
			wantErr: "неизвестное покрытие maybe",
		},
		"manual requires reason": {
			files: map[string]string{
				"openspec/specs/photo-reactions/capability.yaml":     photoMetadata + "  PHOTO-002: manual\n",
				"openspec/specs/photo-reactions/spec.md":             photoLiving,
				"openspec/changes/new/specs/photo-reactions/spec.md": photoDelta,
			},
			wantErr: "PHOTO-002 помечен manual, но причина",
		},
		"delta scenario needs metadata status": {
			files: map[string]string{
				"openspec/specs/photo-reactions/capability.yaml":     photoMetadata,
				"openspec/specs/photo-reactions/spec.md":             photoLiving,
				"openspec/changes/new/specs/photo-reactions/spec.md": photoDelta,
			},
			wantErr: "PHOTO-002 описан в spec.md, но отсутствует в capability.yaml",
		},
		"same capability delta cannot authorize an undescribed ID": {
			files: map[string]string{
				"openspec/specs/photo-reactions/capability.yaml":     photoMetadata + "  PHOTO-002: todo\n  PHOTO-009: todo\n",
				"openspec/specs/photo-reactions/spec.md":             photoLiving,
				"openspec/changes/new/specs/photo-reactions/spec.md": photoDelta,
			},
			wantErr: "PHOTO-009 указан в capability.yaml, но отсутствует",
		},
		"delta capability needs metadata file": {
			files: map[string]string{
				"openspec/changes/new/specs/photo-reactions/spec.md": photoLiving,
			},
			wantErr: "capability.yaml",
		},
		"missing living spec requires same capability delta": {
			files: map[string]string{
				"openspec/specs/photo-reactions/capability.yaml": photoMetadata,
				"openspec/changes/new/specs/unrelated/spec.md":   photoLiving,
			},
			wantErr: "photo-reactions/spec.md",
		},
		"malformed living spec is not replaced by delta": {
			files: map[string]string{
				"openspec/specs/photo-reactions/capability.yaml":     photoMetadata,
				"openspec/specs/photo-reactions/spec.md":             "### Requirement: Photos\n#### Scenario: no ID\n",
				"openspec/changes/new/specs/photo-reactions/spec.md": photoLiving,
			},
			wantErr: "сценарий без идентификатора",
		},
		"invalid prefix": {
			files: map[string]string{
				"openspec/specs/photo-reactions/capability.yaml":     "prefix: photo\nowns: [internal/photo]\nscenarios:\n  PHOTO-001: covered\n",
				"openspec/changes/new/specs/photo-reactions/spec.md": photoLiving,
			},
			wantErr: "префикс photo должен состоять",
		},
		"wrong scenario prefix": {
			files: map[string]string{
				"openspec/specs/photo-reactions/capability.yaml":     "prefix: OTHER\nowns: [internal/photo]\nscenarios:\n  PHOTO-001: covered\n",
				"openspec/changes/new/specs/photo-reactions/spec.md": photoLiving,
			},
			wantErr: "PHOTO-001 не соответствует префиксу OTHER",
		},
		"empty ownership": {
			files: map[string]string{
				"openspec/specs/photo-reactions/capability.yaml":     "prefix: PHOTO\nowns: []\nscenarios:\n  PHOTO-001: covered\n",
				"openspec/changes/new/specs/photo-reactions/spec.md": photoLiving,
			},
			wantErr: "не указан ни один владеемый пакет",
		},
		"new and living capability prefix collision": {
			files: map[string]string{
				"openspec/specs/photo-reactions/capability.yaml":     photoMetadata,
				"openspec/changes/new/specs/photo-reactions/spec.md": photoLiving,
				"openspec/specs/other/capability.yaml":               "prefix: PHOTO\nowns: [internal/other]\nscenarios: {}\n",
				"openspec/specs/other/spec.md":                       "",
			},
			wantErr: "префикс PHOTO занят",
		},
		"new and living capability ownership collision": {
			files: map[string]string{
				"openspec/specs/photo-reactions/capability.yaml":     photoMetadata,
				"openspec/changes/new/specs/photo-reactions/spec.md": photoLiving,
				"openspec/specs/other/capability.yaml":               "prefix: OTHER\nowns: [internal/handlers/on_photo]\nscenarios: {}\n",
				"openspec/specs/other/spec.md":                       "",
			},
			wantErr: "принадлежит сразу двум capability",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			root := readyRoot(t, nil, nil)

			for path, body := range tt.files {
				writeSpecctlFile(t, root, path, body)
			}

			_, err := load(root)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestLoad_ActiveDeltaDoesNotHideLivingSpecReadError(t *testing.T) {
	t.Parallel()

	root := readyRoot(t, nil, nil)
	writeSpecctlFile(t, root, "openspec/specs/photo-reactions/capability.yaml", photoMetadata)
	writeSpecctlFile(t, root, "openspec/changes/new/specs/photo-reactions/spec.md", photoLiving)

	require.NoError(
		t,
		os.MkdirAll(filepath.Join(root, "openspec/specs/photo-reactions/spec.md"), 0o750),
	)

	_, err := load(root)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "is a directory")
}

func writeSpecctlFile(t *testing.T, root, relativePath, body string) {
	t.Helper()

	path := filepath.Join(root, relativePath)

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
}

func TestLoad_SpecWithoutMetadataIsSkippedUnlessDeltaNeedsIt(t *testing.T) {
	t.Parallel()

	root := readyRoot(t, nil, nil)
	writeSpecctlFile(t, root, "openspec/specs/photo-reactions/spec.md", photoLiving)
	loaded, err := load(root)

	require.NoError(t, err)
	assert.Empty(t, loaded.caps)
}

func TestLoad_RejectsSameScenarioInTwoActiveChanges(t *testing.T) {
	t.Parallel()

	root := readyRoot(t, nil, nil)
	writeSpecctlFile(t, root, "openspec/specs/photo-reactions/capability.yaml", photoMetadata)
	writeSpecctlFile(t, root, "openspec/specs/photo-reactions/spec.md", photoLiving)
	writeSpecctlFile(t, root, "openspec/changes/first/specs/photo-reactions/spec.md", photoLiving)
	writeSpecctlFile(t, root, "openspec/changes/second/specs/photo-reactions/spec.md", photoLiving)
	_, err := load(root)

	require.ErrorContains(t, err, "PHOTO-001")
	assert.Contains(t, err.Error(), "first")
	assert.Contains(t, err.Error(), "second")
}
