package on_voice

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gopkg.in/telebot.v3"
)

type Handler struct {
	storage      voicesRepository
	r            randomizer
	logger       logger
	voices       []string
	muVcs        sync.RWMutex
	reactChance  float32
	updatePeriod time.Duration
}

func New(
	ctx context.Context,
	stor voicesRepository,
	logger logger,
	r randomizer,
	reactChance float32,
	updatePeriod time.Duration,
) (*Handler, error) {
	out := Handler{
		storage:      stor,
		logger:       logger,
		r:            r,
		reactChance:  reactChance,
		updatePeriod: updatePeriod,
	}

	if err := out.reloadVoices(); err != nil {
		return nil, fmt.Errorf("cannot load voices: %w", err)
	}

	go out.runUpdater(ctx)

	return &out, nil
}

func (h *Handler) Slug() string {
	return "on_voice"
}

func (h *Handler) Handle(ctx telebot.Context) error {
	if h.r.Float32() > h.reactChance {
		return nil
	}

	h.muVcs.RLock()
	defer h.muVcs.RUnlock()
	randomIndex := h.r.Intn(len(h.voices))
	voice := h.voices[randomIndex]

	if err := ctx.Notify(telebot.RecordingAudio); err != nil {
		return err
	}
	time.Sleep(time.Duration(h.r.Intn(15) * 1_000_000_000))
	err := ctx.Reply(&telebot.Voice{File: telebot.File{FileID: voice}})
	if err != nil {
		h.logger.Error(
			h.logger.WithError(
				h.logger.WithField(
					context.Background(),
					"voice", voice,
				),
				err,
			),
			"can't send voice",
		)
	}
	return err
}
