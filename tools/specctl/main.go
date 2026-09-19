// Команда specctl проверяет, что living specs, код и тесты не разошлись.
//
// Использование:
//
//	specctl verify   — у каждого covered-сценария есть тест с якорем "// spec: <ID>"
//	specctl orphan   — пакеты, не закреплённые ни за одной capability
//	specctl report   — сводка по покрытию и активным changes
//	specctl cover    — сценарии одной capability со статусами
//
// Подробности — SDD.md в корне репозитория.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/reijo1337/ToxicBot/tools/specctl/internal/anchors"
	"github.com/reijo1337/ToxicBot/tools/specctl/internal/changes"
	"github.com/reijo1337/ToxicBot/tools/specctl/internal/check"
	"github.com/reijo1337/ToxicBot/tools/specctl/internal/conf"
	"github.com/reijo1337/ToxicBot/tools/specctl/internal/spec"
	"github.com/reijo1337/ToxicBot/tools/specctl/internal/vcs"
)

const usage = `specctl — гейт соответствия спек, кода и тестов

Команды:
  verify    у каждого covered-сценария есть тест с якорем "// spec: <ID>"
  drift     код capability изменён, а дельты в активном change нет
  coverage  храповик: число непокрытых сценариев не выросло
  orphan    пакеты, не закреплённые ни за одной capability
  report    сводка по покрытию и активным changes
  cover     сценарии одной capability со статусами

Флаги:
  --root    корень репозитория (по умолчанию .)
  --base    ревизия для сравнения в drift и coverage (по умолчанию origin/master)
  --cap     имя capability для команды cover
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "specctl:", err)
		os.Exit(1)
	}
}

// env собирает всё, что нужно проверкам.
type env struct {
	caps    []spec.Capability
	anchors []anchors.Anchor
	active  []changes.Change
	cfg     conf.Config
	root    string
	base    string
	capName string
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)

		return errNoCommand
	}

	command := args[0]

	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	root := fs.String("root", ".", "корень репозитория")
	base := fs.String("base", "origin/master", "ревизия для сравнения")
	capName := fs.String("cap", "", "имя capability для команды cover")

	if err := fs.Parse(args[1:]); err != nil {
		return fmt.Errorf("разбор флагов: %w", err)
	}

	e, err := load(*root)
	if err != nil {
		return err
	}

	e.base = *base
	e.capName = *capName

	return dispatch(context.Background(), command, e)
}

func dispatch(ctx context.Context, command string, e env) error {
	switch command {
	case "verify":
		return emit(check.Verify(e.caps, e.anchors))
	case "drift":
		findings, err := driftFindings(ctx, e)
		if err != nil {
			return err
		}

		return emit(findings)
	case "coverage":
		findings, err := coverageFindings(ctx, e)
		if err != nil {
			return err
		}

		return emit(findings)
	case "orphan":
		packages, err := check.GoPackages(e.root)
		if err != nil {
			return err
		}

		return emit(check.Orphan(e.caps, packages, e.cfg.AllowedOrphans))
	case "report":
		return check.Report(os.Stdout, e.caps, e.anchors, e.active)
	case "cover":
		if e.capName == "" {
			return errNoCapability
		}

		return check.Cover(os.Stdout, e.caps, e.capName)
	default:
		fmt.Fprint(os.Stderr, usage)

		return fmt.Errorf("неизвестная команда %q", command)
	}
}

func load(root string) (env, error) {
	specsDir := filepath.Join(root, "openspec", "specs")

	caps, err := spec.LoadAll(specsDir)
	if err != nil {
		return env{}, err
	}

	found, err := anchors.Scan(root)
	if err != nil {
		return env{}, err
	}

	active, err := changes.Active(filepath.Join(root, "openspec", "changes"))
	if err != nil {
		return env{}, err
	}

	cfg, err := conf.Load(filepath.Join(root, "openspec", "specctl.yaml"))
	if err != nil {
		return env{}, err
	}

	return env{root: root, caps: caps, anchors: found, active: active, cfg: cfg}, nil
}

// emit печатает находки и возвращает ошибку, если среди них есть блокирующие.
func emit(findings []check.Finding) error {
	for _, f := range findings {
		if _, err := fmt.Fprintln(os.Stdout, f); err != nil {
			return fmt.Errorf("печать находок: %w", err)
		}
	}

	if check.HasFailures(findings) {
		return fmt.Errorf("найдено проблем: %d", len(findings))
	}

	if len(findings) == 0 {
		if _, err := fmt.Fprintln(os.Stdout, "чисто"); err != nil {
			return fmt.Errorf("печать результата: %w", err)
		}
	}

	return nil
}

const specsDirRel = "openspec/specs"

// driftFindings сравнивает рабочее дерево с базой и ищет изменения без дельты.
func driftFindings(ctx context.Context, e env) ([]check.Finding, error) {
	repo := vcs.Repo{Dir: e.root}

	from, err := mergeBase(ctx, repo, e.base)
	if err != nil {
		return nil, err
	}

	changed, err := repo.ChangedFiles(ctx, from)
	if err != nil {
		return nil, err
	}

	bodies, err := repo.CommitBodies(ctx, from)
	if err != nil {
		return nil, err
	}

	return check.Drift(e.caps, check.DriftInput{
		Changed:     changed,
		Touched:     changes.TouchedCapabilities(e.active),
		SkipPaths:   e.cfg.SkipDrift,
		SkipReasons: vcs.SkipReasons(bodies),
	}), nil
}

// coverageFindings сравнивает текущий технический долг с тем, что был на базе.
func coverageFindings(ctx context.Context, e env) ([]check.Finding, error) {
	repo := vcs.Repo{Dir: e.root}

	from, err := mergeBase(ctx, repo, e.base)
	if err != nil {
		return nil, err
	}

	base, err := capsAt(ctx, repo, from)
	if err != nil {
		return nil, err
	}

	return check.Coverage(e.caps, base), nil
}

// capsAt читает статусы покрытия из ревизии rev.
func capsAt(ctx context.Context, repo vcs.Repo, rev string) ([]spec.Capability, error) {
	files, err := repo.ListAt(ctx, rev, specsDirRel)
	if err != nil {
		return nil, err
	}

	var out []spec.Capability

	for _, f := range files {
		if filepath.Base(f) != "capability.yaml" {
			continue
		}

		raw, showErr := repo.FileAt(ctx, rev, f)
		if showErr != nil {
			return nil, showErr
		}

		name := filepath.Base(filepath.Dir(f))

		c, parseErr := spec.ParseMeta([]byte(raw), name)
		if parseErr != nil {
			return nil, parseErr
		}

		out = append(out, c)
	}

	return out, nil
}

// mergeBase ищет общего предка с базой и объясняет, что делать, если база недоступна.
// Так бывает в поверхностном клоне CI или когда ветки origin ещё нет.
func mergeBase(ctx context.Context, repo vcs.Repo, base string) (string, error) {
	from, err := repo.MergeBase(ctx, base)
	if err == nil {
		return strings.TrimSpace(from), nil
	}

	return "", fmt.Errorf(
		"не найдена база сравнения %s: нужен полный клон (fetch-depth: 0) "+
			"или укажите другую ревизию через --base: %w",
		base, err,
	)
}
