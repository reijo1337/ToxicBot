package check

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// GoPackages перечисляет каталоги репозитория, содержащие файлы .go, путями относительно root.
func GoPackages(root string) ([]string, error) {
	seen := make(map[string]struct{})

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return skipDir(d.Name())
		}

		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}

		rel, relErr := filepath.Rel(root, filepath.Dir(path))
		if relErr != nil {
			return relErr
		}

		seen[filepath.ToSlash(rel)] = struct{}{}

		return nil
	})
	if err != nil {
		return nil, err
	}

	out := make([]string, 0, len(seen))
	for pkg := range seen {
		out = append(out, pkg)
	}

	sort.Strings(out)

	return out, nil
}

// skipDir отсекает каталоги, которые не относятся к исходникам проекта.
func skipDir(name string) error {
	if name == "vendor" || (strings.HasPrefix(name, ".") && name != "." && name != "..") {
		return filepath.SkipDir
	}

	return nil
}
