package spec

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

const (
	requirementPrefix = "### Requirement:"
	scenarioPrefix    = "#### Scenario:"

	maxLineBytes = 1 << 20
)

// scenarioID выделяет идентификатор вида [PHOTO-003] и остаток заголовка.
var scenarioID = regexp.MustCompile(`^\[([A-Z][A-Z0-9]*-\d{3})\]\s*(.*)$`)

// idPattern — самостоятельный идентификатор сценария без скобок.
var idPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]*-\d{3}$`)

// ValidID сообщает, похожа ли строка на идентификатор сценария.
func ValidID(id string) bool {
	return idPattern.MatchString(id)
}

// ParseFile разбирает spec.md по пути path.
func ParseFile(path string) ([]Scenario, error) {
	f, err := os.Open(path) //nolint:gosec // путь приходит из обхода openspec/specs
	if err != nil {
		return nil, fmt.Errorf("открыть %s: %w", path, err)
	}
	defer f.Close() //nolint:errcheck // только чтение

	return Parse(f, path)
}

// Parse разбирает содержимое living spec. Аргумент name используется в сообщениях об ошибках.
func Parse(r io.Reader, name string) ([]Scenario, error) {
	var (
		out         []Scenario
		requirement string
		seen        = make(map[string]struct{})
		line        int
	)

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), maxLineBytes)

	for sc.Scan() {
		line++

		text := sc.Text()

		if rest, ok := strings.CutPrefix(text, requirementPrefix); ok {
			requirement = strings.TrimSpace(rest)

			continue
		}

		rest, ok := strings.CutPrefix(text, scenarioPrefix)
		if !ok {
			continue
		}

		s, err := parseScenario(strings.TrimSpace(rest), requirement, name, line)
		if err != nil {
			return nil, err
		}

		if _, dup := seen[s.ID]; dup {
			return nil, fmt.Errorf("%s:%d: идентификатор %s уже использован", name, line, s.ID)
		}

		seen[s.ID] = struct{}{}
		out = append(out, s)
	}

	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("чтение %s: %w", name, err)
	}

	return out, nil
}

func parseScenario(head, requirement, name string, line int) (Scenario, error) {
	m := scenarioID.FindStringSubmatch(head)
	if m == nil {
		return Scenario{}, fmt.Errorf(
			"%s:%d: сценарий без идентификатора, ожидается формат [PREFIX-NNN]", name, line,
		)
	}

	if requirement == "" {
		return Scenario{}, fmt.Errorf("%s:%d: сценарий вне требования", name, line)
	}

	return Scenario{
		ID:          m[1],
		Title:       strings.TrimSpace(m[2]),
		Requirement: requirement,
		Line:        line,
	}, nil
}
