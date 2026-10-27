package spec

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"gopkg.in/yaml.v3"
)

const (
	metaFile = "capability.yaml"
	specFile = "spec.md"
)

var prefixPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]*$`)

// Capability — сценарии одной способности бота вместе с метаданными.
type Capability struct {
	Scenarios map[string]Coverage `yaml:"scenarios"`
	Notes     map[string]string   `yaml:"notes"`
	Name      string              `yaml:"-"`
	Prefix    string              `yaml:"prefix"`
	Dir       string              `yaml:"-"`
	Owns      []string            `yaml:"owns"`
	Specs     []Scenario          `yaml:"-"`
}

// DebtCount считает сценарии, оставшиеся техническим долгом.
func (c Capability) DebtCount() int {
	var n int

	for _, cov := range c.Scenarios {
		if cov == CoverageTodo {
			n++
		}
	}

	return n
}

// LoadCapability читает capability.yaml и spec.md из каталога dir и сверяет их между собой.
func LoadCapability(dir string) (Capability, error) {
	return loadCapability(dir, nil)
}

func loadCapability(dir string, deltas []Scenario) (Capability, error) {
	metaPath := filepath.Join(dir, metaFile)

	//nolint:gosec // путь приходит из обхода openspec/specs
	raw, err := os.ReadFile(metaPath)
	if err != nil {
		return Capability{}, fmt.Errorf("читать %s: %w", metaPath, err)
	}

	var c Capability
	if err = yaml.Unmarshal(raw, &c); err != nil {
		return Capability{}, fmt.Errorf("разобрать %s: %w", metaPath, err)
	}

	c.Name = filepath.Base(dir)
	c.Dir = dir

	if c.Specs, err = ParseFile(filepath.Join(dir, specFile)); err != nil {
		if !errors.Is(err, os.ErrNotExist) || len(deltas) == 0 {
			return Capability{}, err
		}
	}

	c.Specs = unionScenarios(c.Specs, deltas)

	if err = c.validate(); err != nil {
		return Capability{}, fmt.Errorf("%s: %w", c.Name, err)
	}

	return c, nil
}

func unionScenarios(living, deltas []Scenario) []Scenario {
	seen := make(map[string]struct{}, len(living)+len(deltas))

	for _, s := range living {
		seen[s.ID] = struct{}{}
	}

	for _, s := range deltas {
		if _, ok := seen[s.ID]; ok {
			continue
		}

		living = append(living, s)
		seen[s.ID] = struct{}{}
	}

	return living
}

func (c Capability) validate() error {
	if !prefixPattern.MatchString(c.Prefix) {
		return fmt.Errorf("префикс %s должен состоять из заглавных латинских букв и цифр", c.Prefix)
	}

	if len(c.Owns) == 0 {
		return errors.New("не указан ни один владеемый пакет")
	}

	if err := c.validateScenarios(); err != nil {
		return err
	}

	return c.validateSymmetry()
}

func (c Capability) validateScenarios() error {
	for id, cov := range c.Scenarios {
		if !cov.Valid() {
			return fmt.Errorf("неизвестное покрытие %s у сценария %s", cov, id)
		}

		if !hasPrefix(id, c.Prefix) {
			return fmt.Errorf("идентификатор %s не соответствует префиксу %s", id, c.Prefix)
		}

		if cov == CoverageManual && c.Notes[id] == "" {
			return fmt.Errorf("сценарий %s помечен manual, но причина в notes не указана", id)
		}
	}

	return nil
}

// validateSymmetry сверяет capability.yaml с объединением living spec и активных дельт.
func (c Capability) validateSymmetry() error {
	inSpec := make(map[string]struct{}, len(c.Specs))

	for _, s := range c.Specs {
		inSpec[s.ID] = struct{}{}

		if _, ok := c.Scenarios[s.ID]; !ok {
			return fmt.Errorf(
				"сценарий %s описан в spec.md, но отсутствует в capability.yaml",
				s.ID,
			)
		}
	}

	for _, id := range sortedKeys(c.Scenarios) {
		if _, ok := inSpec[id]; !ok {
			return fmt.Errorf("сценарий %s указан в capability.yaml, но отсутствует в spec.md", id)
		}
	}

	return nil
}

// LoadAll читает все capability из каталога root, отсортированные по имени.
func LoadAll(root string) ([]Capability, error) {
	return LoadAllWithDeltas(root, nil)
}

// LoadAllWithDeltas сверяет метаданные с living specs и активными дельтами той же capability.
func LoadAllWithDeltas(root string, deltas map[string][]Scenario) ([]Capability, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("читать %s: %w", root, err)
	}

	var out []Capability
	loaded := make(map[string]struct{}, len(entries))

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		dir := filepath.Join(root, e.Name())
		if _, err = os.Stat(filepath.Join(dir, metaFile)); errors.Is(err, os.ErrNotExist) {
			if len(deltas[e.Name()]) == 0 {
				continue
			}
		}

		c, loadErr := loadCapability(dir, deltas[e.Name()])
		if loadErr != nil {
			return nil, loadErr
		}

		out = append(out, c)
		loaded[c.Name] = struct{}{}
	}

	for name := range deltas {
		if _, ok := loaded[name]; !ok {
			return nil, fmt.Errorf("нет capability.yaml для capability %s из активной дельты", name)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})

	if err = checkCollisions(out); err != nil {
		return nil, err
	}

	return out, nil
}

func checkCollisions(caps []Capability) error {
	owners := make(map[string]string)
	prefixes := make(map[string]string)

	for _, c := range caps {
		if prev, ok := prefixes[c.Prefix]; ok {
			return fmt.Errorf("префикс %s занят capability %s и %s", c.Prefix, prev, c.Name)
		}

		prefixes[c.Prefix] = c.Name

		for _, pkg := range c.Owns {
			if prev, ok := owners[pkg]; ok {
				return fmt.Errorf(
					"пакет %s принадлежит сразу двум capability: %s и %s",
					pkg,
					prev,
					c.Name,
				)
			}

			owners[pkg] = c.Name
		}
	}

	return nil
}

func hasPrefix(id, prefix string) bool {
	return len(id) > len(prefix) && id[:len(prefix)] == prefix && id[len(prefix)] == '-'
}

func sortedKeys(m map[string]Coverage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}

	sort.Strings(out)

	return out
}

// ParseMeta разбирает capability.yaml без сверки со spec.md.
// Нужен для чтения прошлого состояния контура из git, где spec.md рядом может уже не быть.
func ParseMeta(raw []byte, name string) (Capability, error) {
	var c Capability
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return Capability{}, fmt.Errorf("разобрать capability.yaml для %s: %w", name, err)
	}

	c.Name = name

	return c, nil
}
