package check

import (
	"fmt"
	"strings"

	"github.com/reijo1337/ToxicBot/tools/specctl/internal/spec"
)

const (
	ruleDrift    = "drift"
	ruleCoverage = "coverage"
)

// DriftInput — всё, что нужно, чтобы решить, разошлись ли код и спеки.
type DriftInput struct {
	// Touched отображает capability на идентификатор change, который несёт на неё дельту.
	Touched map[string]string
	// Changed — файлы, изменённые относительно базы.
	Changed []string
	// SkipPaths — пути, изменение которых не считается изменением поведения.
	SkipPaths []string
	// SkipReasons — причины обхода гейта из тел коммитов.
	SkipReasons []string
}

// Drift ищет изменения кода capability, не подкреплённые дельтой в активном change.
func Drift(caps []spec.Capability, in DriftInput) []Finding {
	out := emptySkipReasons(in.SkipReasons)
	skipped := hasValidSkip(in.SkipReasons)

	severity := SeverityFail
	if skipped {
		severity = SeverityWarn
	}

	for _, c := range caps {
		files := touchedFiles(c, in)
		if len(files) == 0 {
			continue
		}

		if _, ok := in.Touched[c.Name]; ok {
			continue
		}

		out = append(out, Finding{
			Severity: severity,
			Rule:     ruleDrift,
			Location: c.Name,
			Message: fmt.Sprintf(
				"изменены файлы capability (%s), но ни один активный change не несёт на неё дельту",
				joinLimit(files, maxListedFiles),
			),
		})
	}

	return out
}

const (
	maxListedFiles = 3
	testSuffix     = "_test.go"
)

// isTestFile отделяет доказательства от поведения: тест сам по себе ничего не меняет
// для пользователя, поэтому его правка не требует дельты.
func isTestFile(path string) bool {
	return strings.HasSuffix(path, testSuffix)
}

// touchedFiles отбирает изменённые файлы, принадлежащие capability.
func touchedFiles(c spec.Capability, in DriftInput) []string {
	var out []string

	for _, f := range in.Changed {
		if Covers(in.SkipPaths, f) || isTestFile(f) {
			continue
		}

		if Covers(c.Owns, f) {
			out = append(out, f)
		}
	}

	return out
}

// emptySkipReasons требует, чтобы обход гейта всегда назывался своим именем.
func emptySkipReasons(reasons []string) []Finding {
	var out []Finding

	for _, r := range reasons {
		if r == "" {
			out = append(out, Finding{
				Severity: SeverityFail,
				Rule:     ruleDrift,
				Message:  "в коммите есть [skip-spec:] без причины — причина обязательна",
			})
		}
	}

	return out
}

func hasValidSkip(reasons []string) bool {
	for _, r := range reasons {
		if r != "" {
			return true
		}
	}

	return false
}

// Coverage реализует храповик: непокрытых сценариев не должно становиться больше.
// Сравнение идёт по каждой capability отдельно, а появившиеся с нуля не учитываются:
// baseline существующего кода признаёт уже накопленный долг, а не создаёт новый.
func Coverage(current, base []spec.Capability) []Finding {
	was := make(map[string]int, len(base))
	for _, c := range base {
		was[c.Name] = c.DebtCount()
	}

	var out []Finding

	for _, c := range current {
		before, existed := was[c.Name]
		if !existed {
			continue
		}

		now := c.DebtCount()
		if now <= before {
			continue
		}

		out = append(out, Finding{
			Severity: SeverityFail,
			Rule:     ruleCoverage,
			Location: c.Name,
			Message: fmt.Sprintf(
				"непокрытых сценариев стало больше: было %d, стало %d — "+
					"новый или изменённый сценарий должен быть covered или manual",
				before, now,
			),
		})
	}

	return out
}

func joinLimit(items []string, limit int) string {
	if len(items) <= limit {
		return strings.Join(items, ", ")
	}

	return fmt.Sprintf("%s и ещё %d", strings.Join(items[:limit], ", "), len(items)-limit)
}
