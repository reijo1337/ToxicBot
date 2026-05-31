package handlers

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/reijo1337/ToxicBot/pkg/tracing"
	"go.opentelemetry.io/otel/codes"
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
	goCtx, span := tracing.Tracer().Start(context.Background(), tracing.EndpointName(h.endpoint))
	if span.IsRecording() {
		span.SetAttributes(tracing.UpdateAttrs(ctx)...)
	}
	tracing.StashRootContext(ctx, goCtx)
	defer span.End()

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
		span.RecordError(errJoin)
		span.SetStatus(codes.Error, "handler error")
		return fmt.Errorf("got some error from %s handlers: %w", h.endpoint, errJoin)
	}

	return nil
}
