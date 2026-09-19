package check

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/reijo1337/ToxicBot/tools/specctl/internal/anchors"
	"github.com/reijo1337/ToxicBot/tools/specctl/internal/changes"
	"github.com/reijo1337/ToxicBot/tools/specctl/internal/spec"
)

// Stats — сводка покрытия по всем capability.
type Stats struct {
	Covered int
	Manual  int
	Todo    int
}

// Total возвращает общее число сценариев.
func (s Stats) Total() int {
	return s.Covered + s.Manual + s.Todo
}

// Collect считает покрытие по списку capability.
func Collect(caps []spec.Capability) Stats {
	var s Stats

	for _, c := range caps {
		for _, cov := range c.Scenarios {
			switch cov {
			case spec.CoverageCovered:
				s.Covered++
			case spec.CoverageManual:
				s.Manual++
			case spec.CoverageTodo:
				s.Todo++
			}
		}
	}

	return s
}

// Report печатает человекочитаемую сводку состояния контура.
func Report(
	w io.Writer,
	caps []spec.Capability,
	found []anchors.Anchor,
	active []changes.Change,
) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	if _, err := fmt.Fprintln(tw, "CAPABILITY\tCOVERED\tMANUAL\tTODO\tПАКЕТЫ"); err != nil {
		return fmt.Errorf("писать отчёт: %w", err)
	}

	for _, c := range caps {
		s := Collect([]spec.Capability{c})

		_, err := fmt.Fprintf(
			tw,
			"%s\t%d\t%d\t%d\t%s\n",
			c.Name,
			s.Covered,
			s.Manual,
			s.Todo,
			strings.Join(c.Owns, ", "),
		)
		if err != nil {
			return fmt.Errorf("писать отчёт: %w", err)
		}
	}

	if err := tw.Flush(); err != nil {
		return fmt.Errorf("писать отчёт: %w", err)
	}

	return summary(w, Collect(caps), len(found), active)
}

func summary(w io.Writer, s Stats, anchorCount int, active []changes.Change) error {
	lines := []string{
		"",
		fmt.Sprintf("Сценариев: %d (covered %d, manual %d, todo %d)",
			s.Total(), s.Covered, s.Manual, s.Todo),
		fmt.Sprintf("Якорей в тестах: %d", anchorCount),
	}

	if len(active) == 0 {
		lines = append(lines, "Активных changes нет")
	} else {
		lines = append(lines, fmt.Sprintf("Активные changes: %d", len(active)))

		for _, c := range active {
			deltas := "без дельт"
			if len(c.Capabilities) > 0 {
				deltas = strings.Join(c.Capabilities, ", ")
			}

			lines = append(lines, fmt.Sprintf("  %s — %s", c.ID, deltas))
		}
	}

	for _, l := range lines {
		if _, err := fmt.Fprintln(w, l); err != nil {
			return fmt.Errorf("писать отчёт: %w", err)
		}
	}

	return nil
}

// Cover печатает сценарии одной capability со статусами покрытия.
func Cover(w io.Writer, caps []spec.Capability, name string) error {
	for _, c := range caps {
		if c.Name != name {
			continue
		}

		return coverOne(w, c)
	}

	return fmt.Errorf("capability %s не найдена", name)
}

func coverOne(w io.Writer, c spec.Capability) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	if _, err := fmt.Fprintln(tw, "СЦЕНАРИЙ\tСТАТУС\tТРЕБОВАНИЕ\tЗАМЕТКА"); err != nil {
		return fmt.Errorf("писать покрытие: %w", err)
	}

	for _, s := range c.Specs {
		_, err := fmt.Fprintf(
			tw, "%s\t%s\t%s\t%s\n", s.ID, c.Scenarios[s.ID], s.Requirement, c.Notes[s.ID],
		)
		if err != nil {
			return fmt.Errorf("писать покрытие: %w", err)
		}
	}

	if err := tw.Flush(); err != nil {
		return fmt.Errorf("писать покрытие: %w", err)
	}

	stats := Collect([]spec.Capability{c})

	_, err := fmt.Fprintf(
		w, "\n%s: %d сценариев (covered %d, manual %d, todo %d)\n",
		c.Name, stats.Total(), stats.Covered, stats.Manual, stats.Todo,
	)
	if err != nil {
		return fmt.Errorf("писать покрытие: %w", err)
	}

	return nil
}
