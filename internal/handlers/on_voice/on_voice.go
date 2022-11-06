package on_voice

import (
	"context"
	"fmt"
	"github.com/reijo1337/ToxicBot/internal/storage"
	"github.com/sirupsen/logrus"
	"math/rand"
	"sync"
	"time"

	"gopkg.in/telebot.v3"
)

type Handler struct {
	cfg config

	voices []string
	muVcs  sync.RWMutex

	r       *rand.Rand
	storage storage.Manager
	logger  *logrus.Logger
}

func New(ctx context.Context, stor storage.Manager, logger *logrus.Logger) (*Handler, error) {
	out := Handler{
		storage: stor,
		logger:  logger,
		r:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	if err := out.parseConfig(); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := out.reloadVoices(); err != nil {
		return nil, fmt.Errorf("cannot load voices: %w", err)
	}

	go out.runUpdater(ctx)

	return &out, nil
}

func (h *Handler) Handle(ctx telebot.Context) error {
	if h.r.Float32() > h.cfg.ReactChance {
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
	return ctx.Reply(&telebot.Voice{File: telebot.File{FileID: voice}})
}
