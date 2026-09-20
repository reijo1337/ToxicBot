// Команда promptlab — offline-стенд для сравнения вариантов промпта и моделей
// на реальных ситуациях из прода.
//
// Использование:
//
//	promptlab extract      — нарезать копию прод-SQLite на кейсы (JSON)
//	promptlab dump-prompt  — сохранить промпт из кода как стартовый вариант
//	promptlab run          — прогнать матрицу вариант × модель, собрать отчёт
//
// Подробности — README.md рядом.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/reijo1337/ToxicBot/internal/features/message"
	"github.com/reijo1337/ToxicBot/tools/promptlab/internal/cases"
	"github.com/reijo1337/ToxicBot/tools/promptlab/internal/conf"
	"github.com/reijo1337/ToxicBot/tools/promptlab/internal/extract"
	"github.com/reijo1337/ToxicBot/tools/promptlab/internal/lab"
	"github.com/reijo1337/ToxicBot/tools/promptlab/internal/report"
)

const usage = `promptlab — стенд для сравнения промптов и моделей на кейсах из прода

Команды:
  extract      нарезать копию прод-SQLite на кейсы
  dump-prompt  сохранить промпт из кода как стартовый вариант
  run          прогнать матрицу вариант × модель и собрать markdown-отчёт

Флаги каждой команды: promptlab <команда> -h
`

const baselineName = "baseline"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var err error
	switch args[0] {
	case "extract":
		err = runExtract(ctx, args[1:])
	case "dump-prompt":
		err = runDumpPrompt(args[1:])
	case "run":
		err = runMatrix(ctx, args[1:])
	case "-h", "--help", "help":
		fmt.Fprint(os.Stderr, usage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "неизвестная команда %q\n\n%s", args[0], usage)
		return 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "ошибка:", err)
		return 1
	}
	return 0
}

func runExtract(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("extract", flag.ExitOnError)
	dbPath := fs.String("db", "tools/promptlab/data/sas_sqlite.db", "копия прод-базы")
	out := fs.String("out", "tools/promptlab/data/cases.json", "куда писать кейсы")
	minHistory := fs.Int("min-history", 3, "минимум записей в окне перед ответом бота")
	chats := fs.String("chats", "", "только эти chat_id через запятую")
	if err := fs.Parse(args); err != nil {
		return err
	}

	opts := extract.Options{MinHistory: *minHistory}
	if *chats != "" {
		opts.Chats = make(map[int64]struct{})
		for raw := range strings.SplitSeq(*chats, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
			if err != nil {
				return fmt.Errorf("--chats: %w", err)
			}
			opts.Chats[id] = struct{}{}
		}
	}

	cs, err := extract.Run(ctx, *dbPath, opts)
	if err != nil {
		return err
	}
	if err := cases.Save(*out, cs); err != nil {
		return err
	}

	perKind := map[cases.Kind]int{}
	perChat := map[int64]int{}
	for _, c := range cs {
		perKind[c.Kind]++
		perChat[c.ChatID]++
	}
	fmt.Fprintf(os.Stderr, "кейсов: %d (text %d, photo %d) → %s\n",
		len(cs), perKind[cases.KindText], perKind[cases.KindPhoto], *out)
	for id, n := range perChat {
		fmt.Fprintf(os.Stderr, "  чат %d: %d\n", id, n)
	}
	return nil
}

func runDumpPrompt(args []string) error {
	fs := flag.NewFlagSet("dump-prompt", flag.ExitOnError)
	out := fs.String("out", "tools/promptlab/prompts/draft.txt", "куда сохранить промпт из кода")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if _, err := os.Stat(*out); err == nil {
		return fmt.Errorf("%s уже существует, не перезаписываю", *out)
	}
	if err := os.WriteFile(*out, []byte(message.DefaultSystemPromptBase()), 0o600); err != nil {
		return fmt.Errorf("записать %s: %w", *out, err)
	}
	fmt.Fprintf(os.Stderr, "промпт из кода сохранён в %s — правь и запускай run\n", *out)
	return nil
}

func runMatrix(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	casesPath := fs.String("cases", "tools/promptlab/data/cases.json", "файл кейсов")
	configPath := fs.String("config", "tools/promptlab/promptlab.yaml", "конфиг моделей")
	promptsDir := fs.String(
		"prompts",
		"tools/promptlab/prompts",
		"каталог вариантов промпта (*.txt)",
	)
	examplesPath := fs.String(
		"examples",
		"tools/promptlab/data/examples.txt",
		"фразы для блока <examples>, по одной на строку",
	)
	out := fs.String("out", "tools/promptlab/data/report.md", "markdown-отчёт")
	jsonl := fs.String("jsonl", "", "дополнительно сохранить результаты в JSONL")
	models := fs.String("models", "", "только эти модели из конфига, через запятую")
	variants := fs.String(
		"variants", "", "только эти варианты, через запятую (baseline — промпт из кода)",
	)
	limit := fs.Int("limit", 0, "взять только первые N кейсов")
	kind := fs.String("kind", "", "только кейсы этого типа: text или photo")
	noBaseline := fs.Bool("no-baseline", false, "не гонять промпт из кода")
	examplesCount := fs.Int("examples-count", 0, "взять только первые N примеров (0 — все)")
	botHistory := fs.String(
		"bot-history", "all", "реплики бота в истории текстовых кейсов: all, none или last",
	)
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := conf.Load(*configPath)
	if err != nil {
		return err
	}
	selectedModels, err := pickModels(cfg.Models, *models)
	if err != nil {
		return err
	}

	selectedVariants, err := loadVariants(*promptsDir, *variants, !*noBaseline)
	if err != nil {
		return err
	}

	cs, err := cases.Load(*casesPath)
	if err != nil {
		return err
	}
	if *kind != "" {
		filtered := cs[:0]
		for _, c := range cs {
			if string(c.Kind) == *kind {
				filtered = append(filtered, c)
			}
		}
		cs = filtered
	}
	if *limit > 0 && len(cs) > *limit {
		cs = cs[:*limit]
	}
	if len(cs) == 0 {
		return errors.New("после фильтров не осталось кейсов")
	}

	examples, err := readLines(*examplesPath)
	if err != nil {
		return err
	}
	if *examplesCount > 0 && len(examples) > *examplesCount {
		examples = examples[:*examplesCount]
	}
	botHistoryMode, err := lab.ParseBotHistory(*botHistory)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "кейсов %d · вариантов %d · моделей %d · вызовов %d\n",
		len(cs), len(selectedVariants), len(selectedModels),
		len(cs)*len(selectedVariants)*len(selectedModels))

	results, err := lab.Run(ctx, lab.Params{
		Cases:       cs,
		Variants:    selectedVariants,
		Models:      selectedModels,
		Examples:    examples,
		MaxTokens:   cfg.MaxTokens,
		Concurrency: cfg.Concurrency,
		BotHistory:  botHistoryMode,
		Progress:    os.Stderr,
	})
	if err != nil {
		return err
	}

	variantNames := make([]string, 0, len(selectedVariants))
	for _, v := range selectedVariants {
		variantNames = append(variantNames, v.Name)
	}
	modelNames := make([]string, 0, len(selectedModels))
	for _, m := range selectedModels {
		modelNames = append(modelNames, m.Name)
	}

	md := report.Render(cs, variantNames, modelNames, results)
	if err := os.WriteFile(*out, []byte(md), 0o600); err != nil {
		return fmt.Errorf("записать %s: %w", *out, err)
	}
	if *jsonl != "" {
		if err := writeJSONL(*jsonl, results); err != nil {
			return err
		}
	}
	fmt.Fprintf(os.Stderr, "отчёт: %s\n", *out)
	return nil
}

func pickModels(all []conf.Model, filter string) ([]conf.Model, error) {
	if filter == "" {
		return all, nil
	}
	byName := make(map[string]conf.Model, len(all))
	for _, m := range all {
		byName[m.Name] = m
	}
	var out []conf.Model
	for name := range strings.SplitSeq(filter, ",") {
		name = strings.TrimSpace(name)
		m, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("модель %q не описана в конфиге", name)
		}
		out = append(out, m)
	}
	return out, nil
}

// loadVariants: имя варианта — имя файла без .txt; baseline (промпт из кода) идёт первым.
func loadVariants(dir, filter string, withBaseline bool) ([]lab.Variant, error) {
	var out []lab.Variant
	if withBaseline {
		out = append(out, lab.Variant{Name: baselineName})
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.txt"))
	if err != nil {
		return nil, fmt.Errorf("каталог вариантов %s: %w", dir, err)
	}
	for _, f := range files {
		raw, err := os.ReadFile(f) //nolint:gosec // путь из каталога, который задал пользователь
		if err != nil {
			return nil, fmt.Errorf("прочитать %s: %w", f, err)
		}
		name := strings.TrimSuffix(filepath.Base(f), ".txt")
		if name == baselineName {
			return nil, fmt.Errorf(
				"%s: имя %q зарезервировано за промптом из кода",
				f,
				baselineName,
			)
		}
		base := strings.TrimSpace(string(raw))
		if base == "" {
			return nil, fmt.Errorf("%s: пустой вариант", f)
		}
		out = append(out, lab.Variant{Name: name, Base: base})
	}

	if filter == "" {
		if len(out) == 0 {
			return nil, errors.New("нет ни одного варианта промпта")
		}
		return out, nil
	}

	byName := make(map[string]lab.Variant, len(out))
	for _, v := range out {
		byName[v.Name] = v
	}
	var picked []lab.Variant
	for name := range strings.SplitSeq(filter, ",") {
		name = strings.TrimSpace(name)
		v, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("вариант %q не найден в %s", name, dir)
		}
		picked = append(picked, v)
	}
	return picked, nil
}

func readLines(path string) ([]string, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // путь задаёт пользователь инструмента
	if err != nil {
		return nil, fmt.Errorf("прочитать %s: %w", path, err)
	}
	var out []string
	for line := range strings.SplitSeq(string(raw), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf(
			"%s пуст: нужны фразы для блока <examples>, по одной на строку",
			path,
		)
	}
	return out, nil
}

func writeJSONL(path string, results []report.Result) error {
	f, err := os.Create(path) //nolint:gosec // путь задаёт пользователь инструмента
	if err != nil {
		return fmt.Errorf("создать %s: %w", path, err)
	}
	defer f.Close() //nolint:errcheck // ошибки записи ловим ниже

	enc := json.NewEncoder(f)
	for _, r := range results {
		if err := enc.Encode(r); err != nil {
			return fmt.Errorf("записать %s: %w", path, err)
		}
	}
	return nil
}
