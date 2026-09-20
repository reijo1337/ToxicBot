// Package lab гоняет матрицу вариант × модель через настоящий message.Generator:
// envelope, сортировка истории, examples и sanitize идут тем же кодом, что в бою.
package lab

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/reijo1337/ToxicBot/internal/features/message"
	"github.com/reijo1337/ToxicBot/tools/promptlab/internal/cases"
	"github.com/reijo1337/ToxicBot/tools/promptlab/internal/conf"
	"github.com/reijo1337/ToxicBot/tools/promptlab/internal/llm"
	"github.com/reijo1337/ToxicBot/tools/promptlab/internal/report"
)

// Variant — базовая часть системного промпта; пустой Base означает промпт из кода.
type Variant struct {
	Name string
	Base string
}

type Params struct {
	Cases    []cases.Case
	Variants []Variant
	Models   []conf.Model
	// Examples — фразы для блока <examples>; в проде их даёт Google Sheets.
	Examples    []string
	MaxTokens   int64
	Concurrency int
	// BotHistory — реплики бота в окне текстовых кейсов; фото-кейсы всегда без них, как on_photo.
	BotHistory BotHistory
	// Progress — куда писать прогресс; nil = тихо.
	Progress io.Writer
}

type BotHistory string

const (
	BotHistoryAll  BotHistory = "all"
	BotHistoryNone BotHistory = "none"
	BotHistoryLast BotHistory = "last"
)

func ParseBotHistory(s string) (BotHistory, error) {
	switch BotHistory(s) {
	case BotHistoryAll, BotHistoryNone, BotHistoryLast:
		return BotHistory(s), nil
	default:
		return "", fmt.Errorf("bot-history: ожидается all, none или last, получено %q", s)
	}
}

func Run(ctx context.Context, p Params) ([]report.Result, error) {
	if len(p.Examples) == 0 {
		return nil, errors.New("нужен хотя бы один пример для блока <examples>")
	}
	if p.Progress == nil {
		p.Progress = io.Discard
	}

	type pair struct {
		variant Variant
		model   conf.Model
		key     string
	}
	pairs := make([]pair, 0, len(p.Variants)*len(p.Models))
	keys := make(map[string]string, len(p.Models))
	for _, m := range p.Models {
		key := os.Getenv(m.APIKeyEnv)
		if key == "" {
			return nil, fmt.Errorf("модель %s: переменная %s не задана", m.Name, m.APIKeyEnv)
		}
		keys[m.Name] = key
	}
	for _, v := range p.Variants {
		for _, m := range p.Models {
			pairs = append(pairs, pair{variant: v, model: m, key: keys[m.Name]})
		}
	}

	results := make([][]report.Result, len(pairs))
	errs := make([]error, len(pairs))
	sem := make(chan struct{}, max(p.Concurrency, 1))
	var wg sync.WaitGroup
	for i, pr := range pairs {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i], errs[i] = runPair(ctx, p, pr.variant, pr.model, pr.key)
		})
	}
	wg.Wait()

	var out []report.Result
	for i := range pairs {
		if errs[i] != nil {
			return nil, errs[i]
		}
		out = append(out, results[i]...)
	}
	return out, nil
}

func runPair(
	ctx context.Context,
	p Params,
	v Variant,
	m conf.Model,
	apiKey string,
) ([]report.Result, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	maxTokens := p.MaxTokens
	if m.MaxTokens > 0 {
		maxTokens = m.MaxTokens
	}
	client := llm.New(llm.Params{
		BaseURL:     m.BaseURL,
		APIKey:      apiKey,
		Model:       m.Model,
		Temperature: m.Temperature,
		MaxTokens:   maxTokens,
		ExtraBody:   m.ExtraBody,
	})
	rnd := rand.New(rand.NewSource(1)) //nolint:gosec // детерминизм важнее криптостойкости

	gen, err := message.New(
		ctx,
		staticRepo(p.Examples),
		stderrLogger{w: p.Progress},
		rnd,
		alwaysMeaningful{},
		client,
		24*time.Hour,
		message.WithSystemPromptBase(v.Base),
	)
	if err != nil {
		return nil, fmt.Errorf("%s × %s: собрать генератор: %w", v.Name, m.Name, err)
	}

	out := make([]report.Result, 0, len(p.Cases))
	for _, c := range p.Cases {
		history := c.DomainHistory()
		steering := ""
		if c.Kind == cases.KindPhoto {
			// Путь on_photo; факт пересылки в истории не сохраняется, hasForward всегда false.
			history = dropBotEntries(history)
		} else {
			history = applyBotHistory(history, p.BotHistory)
		}
		if c.Kind == cases.KindPhoto {
			trigger, _ := c.Trigger()
			hasCaption := strings.Contains(trigger.Text, "<caption>")
			steering = message.BuildPhotoSteering(rnd, hasCaption, false)
		}

		started := time.Now()
		res := gen.GetMessageTextWithHistoryAndSteering(ctx, history, 1, true, steering)
		r := report.Result{
			CaseID:  c.ID,
			Variant: v.Name,
			Model:   m.Name,
			Latency: time.Since(started),
		}
		switch callErr := client.TakeErr(); {
		case callErr != nil:
			r.Err = callErr.Error()
		case res.Strategy != message.AiGenerationStrategy:
			r.Err = "фоллбэк на список без ошибки LLM (пустая история?)"
		default:
			r.Text = res.Message
		}
		out = append(out, r)
		_, _ = fmt.Fprintf(
			p.Progress,
			"%s × %s · %s · %.1fs\n",
			v.Name,
			m.Name,
			c.ID,
			r.Latency.Seconds(),
		)
	}
	return out, nil
}

func applyBotHistory(history []chathistory.Entry, mode BotHistory) []chathistory.Entry {
	switch mode {
	case BotHistoryNone:
		return dropBotEntries(history)
	case BotHistoryLast:
		lastBot := -1
		for i := len(history) - 1; i >= 0; i-- {
			if history[i].FromBot {
				lastBot = i
				break
			}
		}
		out := make([]chathistory.Entry, 0, len(history))
		for i, e := range history {
			if !e.FromBot || i == lastBot {
				out = append(out, e)
			}
		}
		return out
	default:
		return history
	}
}

func dropBotEntries(history []chathistory.Entry) []chathistory.Entry {
	out := make([]chathistory.Entry, 0, len(history))
	for _, e := range history {
		if !e.FromBot {
			out = append(out, e)
		}
	}
	return out
}

type staticRepo []string

func (r staticRepo) GetEnabledRandom() ([]string, error) { return r, nil }

type alwaysMeaningful struct{}

func (alwaysMeaningful) IsMeaningfulPhrase(string) bool { return true }

type ctxErrKey struct{}

type stderrLogger struct{ w io.Writer }

func (stderrLogger) WithError(ctx context.Context, err error) context.Context {
	return context.WithValue(ctx, ctxErrKey{}, err)
}

func (l stderrLogger) Warn(ctx context.Context, msg string) {
	if err, ok := ctx.Value(ctxErrKey{}).(error); ok {
		_, _ = fmt.Fprintf(l.w, "warn: %s: %v\n", msg, err)
		return
	}
	_, _ = fmt.Fprintf(l.w, "warn: %s\n", msg)
}
