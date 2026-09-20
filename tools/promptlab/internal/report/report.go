// Package report рендерит результаты прогона в markdown: по секции на кейс,
// внутри — контекст, триггер, ответ из прода и таблица вариант × модель.
package report

import (
	"fmt"
	"strings"
	"time"

	"github.com/reijo1337/ToxicBot/tools/promptlab/internal/cases"
)

type Result struct {
	CaseID  string        `json:"case_id"`
	Variant string        `json:"variant"`
	Model   string        `json:"model"`
	Text    string        `json:"text"`
	Err     string        `json:"err,omitempty"`
	Latency time.Duration `json:"latency"`
}

// contextTail — сколько последних записей окна показывать в отчёте.
const contextTail = 6

// Render: порядок variants и models задаёт порядок строк и столбцов таблицы.
func Render(cs []cases.Case, variants, models []string, results []Result) string {
	byKey := make(map[string]Result, len(results))
	for _, r := range results {
		byKey[key(r.CaseID, r.Variant, r.Model)] = r
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# promptlab\n\nКейсов: %d · вариантов: %s · моделей: %s\n",
		len(cs), strings.Join(variants, ", "), strings.Join(models, ", "))

	for i, c := range cs {
		fmt.Fprintf(&b, "\n## %d. `%s` · %s\n\n", i+1, c.ID, c.Kind)

		tail := c.History
		if len(tail) > contextTail {
			tail = tail[len(tail)-contextTail:]
		}
		for j, e := range tail {
			isTrigger := j == len(tail)-1
			marker := ""
			if isTrigger {
				marker = " **← триггер**"
			}
			author := e.Author
			if e.FromBot {
				author = "🤖 " + author
			}
			fmt.Fprintf(
				&b,
				"> `%s` %s: %s%s\n",
				e.Time.UTC().Format("15:04"),
				author,
				cell(e.Text),
				marker,
			)
		}

		fmt.Fprintf(&b, "\n**В проде:** %s\n\n", cell(c.Actual))

		b.WriteString("| вариант \\ модель |")
		for _, m := range models {
			fmt.Fprintf(&b, " %s |", m)
		}
		b.WriteString("\n|---|")
		for range models {
			b.WriteString("---|")
		}
		b.WriteString("\n")

		for _, v := range variants {
			fmt.Fprintf(&b, "| **%s** |", v)
			for _, m := range models {
				r, ok := byKey[key(c.ID, v, m)]
				switch {
				case !ok:
					b.WriteString(" — |")
				case r.Err != "":
					fmt.Fprintf(&b, " ⚠️ %s |", cell(r.Err))
				default:
					fmt.Fprintf(&b, " %s _(%.1fs)_ |", cell(r.Text), r.Latency.Seconds())
				}
			}
			b.WriteString("\n")
		}
	}

	return b.String()
}

func key(caseID, variant, model string) string {
	return caseID + "\x00" + variant + "\x00" + model
}

func cell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}
