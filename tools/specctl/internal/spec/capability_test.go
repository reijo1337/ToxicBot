package spec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeCapability раскладывает capability на диске и возвращает её каталог.
func writeCapability(t *testing.T, root, name, meta, spec string) string {
	t.Helper()

	dir := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, metaFile), []byte(meta), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, specFile), []byte(spec), 0o600))

	return dir
}

const (
	validSpec = `### Requirement: Р
#### Scenario: [PHOTO-001] раз
#### Scenario: [PHOTO-002] два
`

	oneScenarioSpec = "### Requirement: Р\n#### Scenario: [PHOTO-001] раз\n"
)

func TestLoadCapability_OK(t *testing.T) {
	t.Parallel()

	meta := `prefix: PHOTO
owns:
  - internal/handlers/on_photo
scenarios:
  PHOTO-001: covered
  PHOTO-002: manual
notes:
  PHOTO-002: требует живого чата
`
	dir := writeCapability(t, t.TempDir(), "photo-reactions", meta, validSpec)

	got, err := LoadCapability(dir)
	require.NoError(t, err)

	assert.Equal(t, "photo-reactions", got.Name)
	assert.Equal(t, "PHOTO", got.Prefix)
	assert.Equal(t, []string{"internal/handlers/on_photo"}, got.Owns)
	assert.Equal(t, CoverageCovered, got.Scenarios["PHOTO-001"])
	assert.Len(t, got.Specs, 2)
}

func TestLoadCapability_Errors(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		meta    string
		spec    string
		wantErr string
	}{
		"чужой префикс у сценария": {
			meta: `prefix: PHOTO
owns: [internal/handlers/on_photo]
scenarios:
  VOICE-001: covered
`,
			spec:    "### Requirement: Р\n#### Scenario: [VOICE-001] раз\n",
			wantErr: "VOICE-001 не соответствует префиксу PHOTO",
		},
		"manual без причины": {
			meta: `prefix: PHOTO
owns: [internal/handlers/on_photo]
scenarios:
  PHOTO-001: manual
`,
			spec:    oneScenarioSpec,
			wantErr: "PHOTO-001 помечен manual, но причина в notes не указана",
		},
		"неизвестное покрытие": {
			meta: `prefix: PHOTO
owns: [internal/handlers/on_photo]
scenarios:
  PHOTO-001: maybe
`,
			spec:    oneScenarioSpec,
			wantErr: "неизвестное покрытие maybe",
		},
		"сценарий есть в спеке, но не в capability.yaml": {
			meta: `prefix: PHOTO
owns: [internal/handlers/on_photo]
scenarios:
  PHOTO-001: covered
`,
			spec:    validSpec,
			wantErr: "PHOTO-002 описан в spec.md, но отсутствует в capability.yaml",
		},
		"сценарий есть в capability.yaml, но не в спеке": {
			meta: `prefix: PHOTO
owns: [internal/handlers/on_photo]
scenarios:
  PHOTO-001: covered
  PHOTO-009: todo
`,
			spec:    oneScenarioSpec,
			wantErr: "PHOTO-009 указан в capability.yaml, но отсутствует в spec.md",
		},
		"пустой owns": {
			meta: `prefix: PHOTO
owns: []
scenarios:
  PHOTO-001: covered
`,
			spec:    oneScenarioSpec,
			wantErr: "не указан ни один владеемый пакет",
		},
		"кривой префикс": {
			meta: `prefix: photo
owns: [internal/handlers/on_photo]
scenarios: {}
`,
			spec:    "",
			wantErr: "префикс photo должен состоять из заглавных латинских букв",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dir := writeCapability(t, t.TempDir(), "photo-reactions", tt.meta, tt.spec)

			_, err := LoadCapability(dir)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestLoadAll_OverlappingOwns(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeCapability(t, root, "photo-reactions", `prefix: PHOTO
owns: [internal/handlers/on_photo]
scenarios: {}
`, "")
	writeCapability(t, root, "sticker-reactions", `prefix: STICKER
owns: [internal/handlers/on_photo]
scenarios: {}
`, "")

	_, err := LoadAll(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "internal/handlers/on_photo")
	assert.Contains(t, err.Error(), "принадлежит сразу двум capability")
}

func TestLoadAll_DuplicatePrefix(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeCapability(t, root, "photo-reactions", `prefix: PHOTO
owns: [internal/handlers/on_photo]
scenarios: {}
`, "")
	writeCapability(t, root, "photo-extra", `prefix: PHOTO
owns: [internal/handlers/on_voice]
scenarios: {}
`, "")

	_, err := LoadAll(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "префикс PHOTO занят")
}

func TestLoadAll_SortedAndSkipsNonCapabilityDirs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeCapability(t, root, "voice-reactions", `prefix: VOICE
owns: [internal/handlers/on_voice]
scenarios: {}
`, "")
	writeCapability(t, root, "bulling", `prefix: BULL
owns: [internal/handlers/bulling]
scenarios: {}
`, "")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "empty-dir"), 0o750))

	got, err := LoadAll(root)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "bulling", got[0].Name)
	assert.Equal(t, "voice-reactions", got[1].Name)
}
