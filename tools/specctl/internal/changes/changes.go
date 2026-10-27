// Package changes читает активные предложения изменений openspec/changes.
package changes

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/reijo1337/ToxicBot/tools/specctl/internal/spec"
)

const archiveDir = "archive"

// Change — активное (незаархивированное) предложение изменения.
type Change struct {
	DeltaSpecs   map[string][]spec.Scenario
	ID           string
	Dir          string
	Capabilities []string
	// Scenarios — идентификаторы сценариев из дельт: до архивации их нет в living specs.
	Scenarios []string
}

// Active перечисляет активные changes каталога root, отсортированные по идентификатору.
func Active(root string) ([]Change, error) {
	entries, err := os.ReadDir(root)

	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("читать %s: %w", root, err)
	}

	var out []Change
	scenarioOwners := make(map[string]string)

	for _, e := range entries {
		if !e.IsDir() || e.Name() == archiveDir {
			continue
		}

		dir := filepath.Join(root, e.Name())

		caps, capErr := deltaCapabilities(dir)
		if capErr != nil {
			return nil, capErr
		}

		deltas, scErr := deltaScenarios(dir, caps)
		if scErr != nil {
			return nil, scErr
		}

		var scenarios []string

		for _, parsed := range deltas {
			for _, s := range parsed {
				scenarios = append(scenarios, s.ID)
			}
		}

		sort.Strings(scenarios)

		for _, id := range scenarios {
			if owner, exists := scenarioOwners[id]; exists {
				return nil, fmt.Errorf(
					"сценарий %s повторяется в changes %s и %s",
					id,
					owner,
					e.Name(),
				)
			}

			scenarioOwners[id] = e.Name()
		}

		out = append(out, Change{
			ID:           e.Name(),
			Dir:          dir,
			Capabilities: caps,
			Scenarios:    scenarios,
			DeltaSpecs:   deltas,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})

	return out, nil
}

// deltaCapabilities перечисляет capability, на которые change несёт дельту.
func deltaCapabilities(changeDir string) ([]string, error) {
	specsDir := filepath.Join(changeDir, "specs")

	entries, err := os.ReadDir(specsDir)

	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("читать %s: %w", specsDir, err)
	}

	var out []string

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		if _, statErr := os.Stat(filepath.Join(specsDir, e.Name(), "spec.md")); statErr != nil {
			continue
		}

		out = append(out, e.Name())
	}

	sort.Strings(out)

	return out, nil
}

// deltaScenarios сохраняет принадлежность сценариев к capability из пути дельты.
func deltaScenarios(changeDir string, caps []string) (map[string][]spec.Scenario, error) {
	out := make(map[string][]spec.Scenario, len(caps))

	for _, c := range caps {
		parsed, err := spec.ParseFile(filepath.Join(changeDir, "specs", c, "spec.md"))
		if err != nil {
			return nil, err
		}

		out[c] = parsed
	}

	return out, nil
}

// PendingSpecs объединяет сценарии активных дельт по имени capability.
func PendingSpecs(list []Change) map[string][]spec.Scenario {
	out := make(map[string][]spec.Scenario)

	for _, c := range list {
		for name, scenarios := range c.DeltaSpecs {
			out[name] = append(out[name], scenarios...)
		}
	}

	return out
}

// PendingScenarios собирает сценарии из дельт всех активных changes.
func PendingScenarios(list []Change) map[string]struct{} {
	out := make(map[string]struct{})

	for _, c := range list {
		for _, id := range c.Scenarios {
			out[id] = struct{}{}
		}
	}

	return out
}

// TouchedCapabilities отображает capability на идентификатор change, который её меняет.
func TouchedCapabilities(list []Change) map[string]string {
	out := make(map[string]string)

	for _, c := range list {
		for _, capability := range c.Capabilities {
			if _, ok := out[capability]; !ok {
				out[capability] = c.ID
			}
		}
	}

	return out
}
