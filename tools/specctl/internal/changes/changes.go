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

	for _, e := range entries {
		if !e.IsDir() || e.Name() == archiveDir {
			continue
		}

		dir := filepath.Join(root, e.Name())

		caps, capErr := deltaCapabilities(dir)
		if capErr != nil {
			return nil, capErr
		}

		scenarios, scErr := deltaScenarios(dir, caps)
		if scErr != nil {
			return nil, scErr
		}

		out = append(out, Change{ID: e.Name(), Dir: dir, Capabilities: caps, Scenarios: scenarios})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })

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

// deltaScenarios собирает идентификаторы сценариев из дельт перечисленных capability.
func deltaScenarios(changeDir string, caps []string) ([]string, error) {
	var out []string

	for _, c := range caps {
		parsed, err := spec.ParseFile(filepath.Join(changeDir, "specs", c, "spec.md"))
		if err != nil {
			return nil, err
		}

		for _, s := range parsed {
			out = append(out, s.ID)
		}
	}

	sort.Strings(out)

	return out, nil
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
