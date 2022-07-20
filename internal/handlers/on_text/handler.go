package on_text

import (
	"fmt"
	"strings"
	"sync"

	"gopkg.in/telebot.v3"
)

type SubHandler interface {
	Slug() string
	Handle(telebot.Context) error
}

type Handler struct {
	handlers []SubHandler
}

type MotherError map[string]error

func (m MotherError) Error() string {
	result := strings.Builder{}

	for slug, err := range m {
		result.WriteRune('\n')
		result.WriteString(slug)
		result.WriteString(": ")
		result.WriteString(err.Error())
	}

	return result.String()
}

func New(h ...SubHandler) *Handler {
	return &Handler{
		handlers: h,
	}
}

func (h *Handler) Handle(ctx telebot.Context) error {
	errMu := sync.Mutex{}
	motherError := make(MotherError)

	wg := sync.WaitGroup{}
	wg.Add(len(h.handlers))

	for _, h := range h.handlers {
		go func(h SubHandler) {
			defer wg.Done()
			if err := h.Handle(ctx); err != nil {
				errMu.Lock()
				motherError[h.Slug()] = err
				errMu.Unlock()
			}
		}(h)
	}

	wg.Wait()

	if len(motherError) != 0 {
		return fmt.Errorf("got some error from on_text handlers: %w", motherError)
	}

	return nil
}
