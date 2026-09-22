// Package cases описывает кейс для прогона: окно истории чата, в конце которого
// бот в проде на что-то ответил. Ответ из прода сохраняется как точка отсчёта.
package cases

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
)

// Kind — тип триггера; фото распознаётся по PreFormatted-записи от on_photo.
type Kind string

const (
	KindText  Kind = "text"
	KindPhoto Kind = "photo"
)

// Entry — запись истории в JSON-фикстуре. Дублирует chathistory.Entry, чтобы
// фикстура имела стабильные snake_case-поля, не зависящие от доменного типа.
type Entry struct {
	ID           int       `json:"id"`
	Time         time.Time `json:"time"`
	Author       string    `json:"author"`
	Text         string    `json:"text"`
	ReplyToID    int       `json:"reply_to_id,omitempty"`
	FromBot      bool      `json:"from_bot,omitempty"`
	PreFormatted bool      `json:"pre_formatted,omitempty"`
}

type Case struct {
	ID      string  `json:"id"`
	ChatID  int64   `json:"chat_id"`
	Kind    Kind    `json:"kind"`
	History []Entry `json:"history"`
	Actual  string  `json:"actual"`
}

// Trigger — последняя реплика человека в окне; именно на неё бот отвечал.
func (c Case) Trigger() (Entry, bool) {
	for i := len(c.History) - 1; i >= 0; i-- {
		if !c.History[i].FromBot {
			return c.History[i], true
		}
	}
	return Entry{}, false
}

func (c Case) DomainHistory() []chathistory.Entry {
	out := make([]chathistory.Entry, 0, len(c.History))
	for _, e := range c.History {
		out = append(out, chathistory.Entry{
			ID:           e.ID,
			Time:         e.Time,
			Author:       e.Author,
			Text:         e.Text,
			ReplyToID:    e.ReplyToID,
			FromBot:      e.FromBot,
			PreFormatted: e.PreFormatted,
		})
	}
	return out
}

// FromHistory: кейс — каждая запись бота, перед которой не меньше minHistory записей
// и хотя бы одна реплика человека. Сортировка по времени и ID повторяет генератор.
func FromHistory(chatID int64, entries []chathistory.Entry, minHistory int) []Case {
	sorted := make([]chathistory.Entry, len(entries))
	copy(sorted, entries)
	sort.SliceStable(sorted, func(i, j int) bool {
		if !sorted[i].Time.Equal(sorted[j].Time) {
			return sorted[i].Time.Before(sorted[j].Time)
		}
		return sorted[i].ID < sorted[j].ID
	})

	var out []Case
	for i, e := range sorted {
		if !e.FromBot || i < minHistory {
			continue
		}
		window := sorted[:i]
		c := Case{
			ID:      fmt.Sprintf("%d-%d", chatID, e.ID),
			ChatID:  chatID,
			History: make([]Entry, 0, len(window)),
			Actual:  e.Text,
		}
		for _, h := range window {
			c.History = append(c.History, Entry{
				ID:           h.ID,
				Time:         h.Time,
				Author:       h.Author,
				Text:         h.Text,
				ReplyToID:    h.ReplyToID,
				FromBot:      h.FromBot,
				PreFormatted: h.PreFormatted,
			})
		}
		trigger, ok := c.Trigger()
		if !ok {
			continue
		}
		c.Kind = KindText
		if trigger.PreFormatted && strings.Contains(trigger.Text, "<photo>") {
			c.Kind = KindPhoto
		}
		out = append(out, c)
	}
	return out
}

func Save(path string, cs []Case) error {
	raw, err := json.MarshalIndent(cs, "", "  ")
	if err != nil {
		return fmt.Errorf("сериализовать кейсы: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("записать %s: %w", path, err)
	}
	return nil
}

func Load(path string) ([]Case, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // путь задаёт пользователь инструмента
	if err != nil {
		return nil, fmt.Errorf("прочитать %s: %w", path, err)
	}
	var cs []Case
	if err := json.Unmarshal(raw, &cs); err != nil {
		return nil, fmt.Errorf("разобрать %s: %w", path, err)
	}
	return cs, nil
}
