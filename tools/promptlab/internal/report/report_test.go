package report

import (
	"strings"
	"testing"
	"time"

	"github.com/reijo1337/ToxicBot/tools/promptlab/internal/cases"
	"github.com/stretchr/testify/assert"
)

const (
	caseID   = "1-3"
	model    = "ds"
	baseline = "baseline"
)

func TestRender_TableAndContext(t *testing.T) {
	t.Parallel()

	cs := []cases.Case{{
		ID:     caseID,
		ChatID: 1,
		Kind:   cases.KindText,
		History: []cases.Entry{
			{
				ID:     1,
				Time:   time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
				Author: "@alice",
				Text:   "привет | всем",
			},
			{
				ID:      2,
				Time:    time.Date(2026, 9, 1, 12, 1, 0, 0, time.UTC),
				Author:  "бот",
				Text:    "отвали",
				FromBot: true,
			},
			{
				ID:     3,
				Time:   time.Date(2026, 9, 1, 12, 2, 0, 0, time.UTC),
				Author: "@bob",
				Text:   "грубо",
			},
		},
		Actual: "и что",
	}}
	results := []Result{
		{
			CaseID:  caseID,
			Variant: baseline,
			Model:   model,
			Text:    "сам грубый",
			Latency: 1500 * time.Millisecond,
		},
		{CaseID: caseID, Variant: baseline, Model: "grok", Err: "boom"},
		{
			CaseID:  caseID,
			Variant: "v2",
			Model:   model,
			Text:    "ответ\nв две строки",
			Latency: time.Second,
		},
	}

	md := Render(cs, []string{baseline, "v2"}, []string{model, "grok"}, results)

	assert.Contains(t, md, "## 1. `1-3` · text")
	assert.Contains(t, md, "> `12:00` @alice: привет \\| всем")
	assert.Contains(t, md, "🤖 бот: отвали")
	assert.Contains(t, md, "@bob: грубо **← триггер**")
	assert.Contains(t, md, "**В проде:** и что")
	assert.Contains(t, md, "| вариант \\ модель | ds | grok |")
	assert.Contains(t, md, "| **baseline** | сам грубый _(1.5s)_ | ⚠️ boom |")
	assert.Contains(t, md, "| **v2** | ответ в две строки _(1.0s)_ | — |")
}

func TestRender_ContextTailIsBounded(t *testing.T) {
	t.Parallel()

	c := cases.Case{ID: "1-99", Kind: cases.KindText}
	for i := 1; i < 20; i++ {
		c.History = append(
			c.History,
			cases.Entry{ID: i, Author: "@a", Text: "m" + strings.Repeat("!", i)},
		)
	}

	md := Render([]cases.Case{c}, []string{baseline}, []string{model}, nil)

	assert.Equal(t, contextTail, strings.Count(md, "\n> "))
	assert.NotContains(t, md, "@a: m!\n")
}

func TestRenderTagTargetAndEveryRepetition(t *testing.T) {
	t.Parallel()
	cs := []cases.Case{
		{
			ID:     "synthetic-tag",
			Kind:   cases.KindTag,
			Target: &cases.Target{UserID: 901, Author: "@synthetic_target", Nickname: "Чайник"},
			History: []cases.Entry{
				{ID: 1, SenderID: 902, Author: "@synthetic_other", Text: "Общий фон"},
				{ID: 2, FromBot: true, Text: "Ошибка старого бота"},
			},
		},
	}
	results := []Result{
		{
			CaseID:     "synthetic-tag",
			Variant:    baseline,
			Model:      model,
			Repetition: 1,
			Text:       "Первый уникальный ответ",
		},
		{
			CaseID:     "synthetic-tag",
			Variant:    baseline,
			Model:      model,
			Repetition: 2,
			Err:        "уникальная ошибка API",
		},
		{
			CaseID:     "synthetic-tag",
			Variant:    baseline,
			Model:      model,
			Repetition: 3,
			Text:       "Третий уникальный ответ",
		},
	}
	md := Render(cs, []string{baseline}, []string{model}, results)
	assert.Contains(t, md, "**Адресат:** @synthetic_target · ID `901` · кличка: Чайник")
	assert.Contains(t, md, "@synthetic_other (sender_id=902): Общий фон")
	assert.NotContains(t, md, "триггер")
	assert.NotContains(t, md, "**В проде:**")
	for _, text := range []string{"Первый уникальный ответ", "уникальная ошибка API", "Третий уникальный ответ"} {
		assert.Equal(t, 1, strings.Count(md, text))
	}
	assert.Contains(t, md, "повтор 1")
	assert.Contains(t, md, "повтор 2")
	assert.Contains(t, md, "повтор 3")
}
