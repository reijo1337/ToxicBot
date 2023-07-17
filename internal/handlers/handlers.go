package handlers

import (
	"errors"
	"fmt"
	"sync"

	"gopkg.in/telebot.v3"
)

type Handler struct {
	endpoint string
	handlers []subHandler
}

func New(endpoint string, h ...subHandler) *Handler {
	return &Handler{
		endpoint: endpoint,
		handlers: h,
	}
}

func (h *Handler) Handle(ctx telebot.Context) error {
	wg := sync.WaitGroup{}
	wg.Add(len(h.handlers))

	errs := make(chan error, len(h.handlers))

	for _, h := range h.handlers {
		go func(h subHandler) {
			defer wg.Done()
			if err := h.Handle(ctx); err != nil {
				errs <- err
			}
		}(h)
	}

	go func() {
		wg.Wait()
		close(errs)
	}()

	var errJoin error
	for err := range errs {
		errJoin = errors.Join(errJoin, err)
	}

	if errJoin != nil {
		return fmt.Errorf("got some error from %s handlers: %w", h.endpoint, errJoin)
	}

	return nil
}
