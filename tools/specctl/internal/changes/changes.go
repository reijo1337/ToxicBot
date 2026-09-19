// Package changes читает активные предложения изменений openspec/changes.
package changes

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

const archiveDir = "archive"

// Change — активное (незаархивированное) предложение изменения.
type Change struct {
	ID           string
	Dir          string
	Capabilities []string
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

		out = append(out, Change{ID: e.Name(), Dir: dir, Capabilities: caps})
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
