// Package vcs достаёт из git то, что нужно гейту: изменённые файлы и тела коммитов.
package vcs

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const gitTimeout = 30 * time.Second

// skipPattern выделяет обход гейта: [skip-spec: причина].
// Директива обязана занимать отдельную строку целиком, иначе любое упоминание
// синтаксиса в тексте коммита молча отключало бы проверку.
var skipPattern = regexp.MustCompile(`(?m)^[ \t]*\[skip-spec:([^\]]*)\][ \t]*$`)

// Repo — рабочая копия, к которой обращается гейт.
type Repo struct {
	Dir string
}

// MergeBase возвращает общего предка ветки и base.
func (r Repo) MergeBase(ctx context.Context, base string) (string, error) {
	out, err := r.git(ctx, "merge-base", base, "HEAD")
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(out), nil
}

// ChangedFiles перечисляет файлы, изменённые между from и рабочим деревом.
func (r Repo) ChangedFiles(ctx context.Context, from string) ([]string, error) {
	out, err := r.git(ctx, "diff", "--name-only", from)
	if err != nil {
		return nil, err
	}

	return nonEmptyLines(out), nil
}

// CommitBodies возвращает полные сообщения коммитов после from.
func (r Repo) CommitBodies(ctx context.Context, from string) ([]string, error) {
	// NUL в аргументах exec недопустим, поэтому разделяем записи символом RS.
	const sep = "\x1e"

	out, err := r.git(ctx, "log", "--format=%B"+sep, from+"..HEAD")
	if err != nil {
		return nil, err
	}

	var bodies []string

	for _, b := range strings.Split(out, sep) {
		if s := strings.TrimSpace(b); s != "" {
			bodies = append(bodies, s)
		}
	}

	return bodies, nil
}

func (r Repo) git(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()

	// G204: аргументы формирует сам specctl, пользовательский ввод сюда не попадает.
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec
	cmd.Dir = r.Dir

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}

	return string(out), nil
}

// SkipReasons достаёт причины обхода гейта из тел коммитов.
// Пустая причина возвращается как пустая строка — вызывающий решает, что с ней делать.
func SkipReasons(bodies []string) []string {
	var out []string

	for _, b := range bodies {
		for _, m := range skipPattern.FindAllStringSubmatch(b, -1) {
			out = append(out, strings.TrimSpace(m[1]))
		}
	}

	return out
}

func nonEmptyLines(s string) []string {
	var out []string

	for _, l := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(l); t != "" {
			out = append(out, t)
		}
	}

	return out
}

// ListAt перечисляет файлы каталога dir в ревизии rev.
// Отсутствие каталога не считается ошибкой — возвращается пустой список.
func (r Repo) ListAt(ctx context.Context, rev, dir string) ([]string, error) {
	out, err := r.git(ctx, "ls-tree", "-r", "--name-only", rev, "--", dir)
	if err != nil {
		return nil, err
	}

	return nonEmptyLines(out), nil
}

// FileAt читает содержимое файла в ревизии rev.
func (r Repo) FileAt(ctx context.Context, rev, path string) (string, error) {
	return r.git(ctx, "show", rev+":"+path)
}
