package spec

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_ScenarioIDAndTitle(t *testing.T) {
	t.Parallel()

	src := `# Реакции на фото

## Requirements

### Requirement: Бот не повторяет собственные реплики
Бот SHALL исключать свои сообщения из истории.

#### Scenario: [PHOTO-003] последнее сообщение от бота
- GIVEN история заканчивается репликой бота
- WHEN приходит фото
- THEN промпт собирается без записей бота
`

	got, err := Parse(strings.NewReader(src), "spec.md")
	require.NoError(t, err)
	require.Len(t, got, 1)

	assert.Equal(t, "PHOTO-003", got[0].ID)
	assert.Equal(t, "последнее сообщение от бота", got[0].Title)
	assert.Equal(t, "Бот не повторяет собственные реплики", got[0].Requirement)
	assert.Equal(t, 8, got[0].Line)
}

func TestParse_MultipleRequirements(t *testing.T) {
	t.Parallel()

	src := `### Requirement: Первое
#### Scenario: [CAP-001] раз
### Requirement: Второе
#### Scenario: [CAP-002] два
#### Scenario: [CAP-003] три
`

	got, err := Parse(strings.NewReader(src), "spec.md")
	require.NoError(t, err)
	require.Len(t, got, 3)

	assert.Equal(t, "Первое", got[0].Requirement)
	assert.Equal(t, "Второе", got[1].Requirement)
	assert.Equal(t, "Второе", got[2].Requirement)
}

func TestParse_Errors(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		src     string
		wantErr string
	}{
		"сценарий без идентификатора": {
			src: `### Requirement: Р
#### Scenario: просто текст
`,
			wantErr: "spec.md:2: сценарий без идентификатора",
		},
		"дублирующийся идентификатор": {
			src: `### Requirement: Р
#### Scenario: [CAP-001] раз
#### Scenario: [CAP-001] два
`,
			wantErr: "spec.md:3: идентификатор CAP-001 уже использован",
		},
		"сценарий вне требования": {
			src: `#### Scenario: [CAP-001] раз
`,
			wantErr: "spec.md:1: сценарий вне требования",
		},
		"кривой формат идентификатора": {
			src: `### Requirement: Р
#### Scenario: [photo-3] раз
`,
			wantErr: "spec.md:2: сценарий без идентификатора",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := Parse(strings.NewReader(tt.src), "spec.md")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestParse_IgnoresProse(t *testing.T) {
	t.Parallel()

	src := `# Заголовок

Обычный текст про #### Scenario: не считается.

### Requirement: Р
Текст требования.

#### Scenario: [CAP-001] настоящий
- GIVEN что-то
`

	got, err := Parse(strings.NewReader(src), "spec.md")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "CAP-001", got[0].ID)
}
