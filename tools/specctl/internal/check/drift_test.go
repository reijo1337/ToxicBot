package check

import (
	"testing"

	"github.com/reijo1337/ToxicBot/tools/specctl/internal/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func driftCaps() []spec.Capability {
	return []spec.Capability{{Name: photoCapNm, Owns: []string{photoPkg}}}
}

func TestDrift_CodeWithoutDelta(t *testing.T) {
	t.Parallel()

	got := Drift(driftCaps(), DriftInput{Changed: []string{photoPkg + "/on_photo.go"}})
	require.Len(t, got, 1)
	assert.Equal(t, SeverityFail, got[0].Severity)
	assert.Equal(t, photoCapNm, got[0].Location)
	assert.Contains(t, got[0].Message, "on_photo.go")
}

func TestDrift_DeltaPresent(t *testing.T) {
	t.Parallel()

	got := Drift(driftCaps(), DriftInput{
		Changed: []string{photoPkg + "/on_photo.go"},
		Touched: map[string]string{photoCapNm: "fix-photo-loop"},
	})

	assert.Empty(t, got)
}

func TestDrift_SkipPathsIgnored(t *testing.T) {
	t.Parallel()

	got := Drift(driftCaps(), DriftInput{
		Changed:   []string{"docs/SDD.md", "deploy/deploy.yaml"},
		SkipPaths: []string{"docs", "deploy"},
	})

	assert.Empty(t, got)
}

func TestDrift_SkipSpecDowngradesToWarn(t *testing.T) {
	t.Parallel()

	got := Drift(driftCaps(), DriftInput{
		Changed:     []string{photoPkg + "/on_photo.go"},
		SkipReasons: []string{"хотфикс на проде"},
	})

	require.Len(t, got, 1)
	assert.Equal(t, SeverityWarn, got[0].Severity)
	assert.False(t, HasFailures(got))
}

func TestDrift_EmptySkipReasonFails(t *testing.T) {
	t.Parallel()

	got := Drift(driftCaps(), DriftInput{
		Changed:     []string{photoPkg + "/on_photo.go"},
		SkipReasons: []string{""},
	})

	require.Len(t, got, 2)
	assert.True(t, HasFailures(got))
	assert.Contains(t, got[0].Message, "без причины")
}

func TestDrift_UnownedFilesIgnored(t *testing.T) {
	t.Parallel()

	got := Drift(driftCaps(), DriftInput{Changed: []string{"internal/config/config.go"}})
	assert.Empty(t, got)
}

func TestCoverage_Ratchet(t *testing.T) {
	t.Parallel()

	withTodo := func(n int) []spec.Capability {
		scen := make(map[string]spec.Coverage, n)
		for i := range n {
			scen[string(rune('A'+i))] = spec.CoverageTodo
		}

		return []spec.Capability{{Name: photoCapNm, Scenarios: scen}}
	}

	assert.Empty(t, Coverage(withTodo(2), withTodo(3)), "долг уменьшился")
	assert.Empty(t, Coverage(withTodo(3), withTodo(3)), "долг не изменился")

	got := Coverage(withTodo(4), withTodo(3))
	require.Len(t, got, 1)
	assert.Equal(t, SeverityFail, got[0].Severity)
	assert.Equal(t, photoCapNm, got[0].Location)
	assert.Contains(t, got[0].Message, "было 3, стало 4")
}

func TestCoverage_NewCapabilityBringsItsDebtLegally(t *testing.T) {
	t.Parallel()

	fresh := []spec.Capability{
		{
			Name: "tagger",
			Scenarios: map[string]spec.Coverage{
				"TAG-001": spec.CoverageTodo,
				"TAG-002": spec.CoverageTodo,
			},
		},
	}

	assert.Empty(t, Coverage(fresh, nil), "baseline признаёт накопленный долг, а не создаёт новый")
}

func TestCoverage_DebtCannotMoveBetweenCapabilities(t *testing.T) {
	t.Parallel()

	base := []spec.Capability{
		{Name: "a", Scenarios: map[string]spec.Coverage{"A-001": spec.CoverageTodo}},
		{Name: "b", Scenarios: map[string]spec.Coverage{"B-001": spec.CoverageCovered}},
	}
	current := []spec.Capability{
		{Name: "a", Scenarios: map[string]spec.Coverage{"A-001": spec.CoverageCovered}},
		{Name: "b", Scenarios: map[string]spec.Coverage{
			"B-001": spec.CoverageTodo,
			"B-002": spec.CoverageTodo,
		}},
	}

	got := Coverage(current, base)
	require.Len(t, got, 1, "суммарно долг тот же, но в b он вырос")
	assert.Equal(t, "b", got[0].Location)
}
