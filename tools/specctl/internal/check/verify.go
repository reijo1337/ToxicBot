package check

import (
	"fmt"
	"sort"
	"strings"

	"github.com/reijo1337/ToxicBot/tools/specctl/internal/anchors"
	"github.com/reijo1337/ToxicBot/tools/specctl/internal/spec"
)

const ruleVerify = "verify"

// Verify сверяет статусы покрытия сценариев с якорями в тестах.
func Verify(caps []spec.Capability, found []anchors.Anchor) []Finding {
	anchored := make(map[string][]anchors.Anchor, len(found))
	for _, a := range found {
		anchored[a.ScenarioID] = append(anchored[a.ScenarioID], a)
	}

	known := make(map[string]struct{})

	var out []Finding

	for _, c := range caps {
		for _, id := range sortedIDs(c.Scenarios) {
			known[id] = struct{}{}

			if c.Scenarios[id] != spec.CoverageCovered {
				continue
			}

			if len(anchored[id]) == 0 {
				out = append(out, Finding{
					Severity: SeverityFail,
					Rule:     ruleVerify,
					Location: c.Name,
					Message: fmt.Sprintf(
						"сценарий %s помечен covered, но нет теста с якорем \"// spec: %s\"",
						id,
						id,
					),
				})
			}
		}
	}

	return append(out, danglingAnchors(found, known)...)
}

// danglingAnchors ловит якоря, указывающие на несуществующие сценарии.
func danglingAnchors(found []anchors.Anchor, known map[string]struct{}) []Finding {
	var out []Finding

	for _, a := range found {
		if _, ok := known[a.ScenarioID]; ok {
			continue
		}

		out = append(out, Finding{
			Severity: SeverityFail,
			Rule:     ruleVerify,
			Location: fmt.Sprintf("%s:%d", a.File, a.Line),
			Message: fmt.Sprintf(
				"тест %s ссылается на сценарий %s, которого не существует",
				a.TestName,
				a.ScenarioID,
			),
		})
	}

	return out
}

const ruleOrphan = "orphan"

// Orphan ищет пакеты, не закреплённые ни за одной capability.
func Orphan(caps []spec.Capability, packages, allowed []string) []Finding {
	var owned []string
	for _, c := range caps {
		owned = append(owned, c.Owns...)
	}

	var out []Finding

	for _, pkg := range packages {
		if touches(owned, pkg) || touches(allowed, pkg) {
			continue
		}

		out = append(out, Finding{
			Severity: SeverityWarn,
			Rule:     ruleOrphan,
			Location: pkg,
			Message:  "пакет не принадлежит ни одной capability",
		})
	}

	return out
}

// Covers сообщает, попадает ли путь под одну из записей владения.
// Запись может быть каталогом (internal/handlers/on_photo) или конкретным файлом.
func Covers(owned []string, path string) bool {
	for _, p := range owned {
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}

	return false
}

// touches проверяет связь пакета с владением в обе стороны: пакет может лежать внутри
// владеемого каталога, а владеемый файл — внутри пакета.
func touches(owned []string, pkg string) bool {
	if Covers(owned, pkg) {
		return true
	}

	for _, p := range owned {
		if strings.HasPrefix(p, pkg+"/") {
			return true
		}
	}

	return false
}

func sortedIDs(m map[string]spec.Coverage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}

	sort.Strings(out)

	return out
}
