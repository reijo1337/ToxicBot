package on_voice

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/reijo1337/ToxicBot/internal/storage"
	"github.com/sirupsen/logrus"
	"gopkg.in/telebot.v3"
)

type Handler struct {
	storage storage.Manager
	r       *rand.Rand
	logger  *logrus.Logger
	voices  []string
	cfg     config
	muVcs   sync.RWMutex
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
	err := ctx.Reply(&telebot.Voice{File: telebot.File{FileID: voice}})
	if err != nil {
		h.logger.WithError(err).WithField("voice", voice).Error("can't send voice")
	}
	return err
}
