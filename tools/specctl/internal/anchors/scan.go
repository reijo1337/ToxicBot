// Package anchors ищет в тестах якоря вида "// spec: PHOTO-003".
package anchors

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/reijo1337/ToxicBot/tools/specctl/internal/spec"
)

const (
	anchorPrefix = "// spec:"
	testPrefix   = "Test"
	testSuffix   = "_test.go"
)

// Anchor связывает сценарий living spec с конкретным тестом.
type Anchor struct {
	ScenarioID string
	TestName   string
	File       string
	Line       int
}

// Scan обходит root и собирает якоря из всех файлов *_test.go.
func Scan(root string) ([]Anchor, error) {
	var out []Anchor

	fset := token.NewFileSet()

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return skipDir(d.Name())
		}

		if !strings.HasSuffix(d.Name(), testSuffix) {
			return nil
		}

		found, parseErr := scanFile(fset, path)
		if parseErr != nil {
			return parseErr
		}

		out = append(out, found...)

		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].ScenarioID != out[j].ScenarioID {
			return out[i].ScenarioID < out[j].ScenarioID
		}

		return out[i].File < out[j].File
	})

	return out, nil
}

// skipDir отсекает служебные каталоги, где искать тесты бессмысленно.
func skipDir(name string) error {
	if name == "vendor" || (strings.HasPrefix(name, ".") && name != "." && name != "..") {
		return filepath.SkipDir
	}

	return nil
}

func scanFile(fset *token.FileSet, path string) ([]Anchor, error) {
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("разобрать %s: %w", path, err)
	}

	var out []Anchor

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Doc == nil || !isTestFunc(fn) {
			continue
		}

		line := fset.Position(fn.Pos()).Line

		ids, idErr := anchorIDs(fn.Doc, path, fset)
		if idErr != nil {
			return nil, idErr
		}

		for _, id := range ids {
			out = append(out, Anchor{
				ScenarioID: id,
				TestName:   fn.Name.Name,
				File:       path,
				Line:       line,
			})
		}
	}

	return out, nil
}

func anchorIDs(doc *ast.CommentGroup, path string, fset *token.FileSet) ([]string, error) {
	var ids []string

	for _, c := range doc.List {
		rest, ok := strings.CutPrefix(c.Text, anchorPrefix)
		if !ok {
			continue
		}

		id := strings.TrimSpace(rest)
		if !spec.ValidID(id) {
			return nil, fmt.Errorf(
				"%s:%d: якорь %q не похож на идентификатор сценария, ожидается формат [PREFIX-NNN]",
				path, fset.Position(c.Pos()).Line, id,
			)
		}

		ids = append(ids, id)
	}

	return ids, nil
}

func isTestFunc(fn *ast.FuncDecl) bool {
	return fn.Recv == nil && strings.HasPrefix(fn.Name.Name, testPrefix)
}
