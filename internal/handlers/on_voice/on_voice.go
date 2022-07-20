package on_voice

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/reijo1337/ToxicBot/internal/utils"
	"gopkg.in/telebot.v3"
)

type Handler struct {
	cfg      config
	stickers []string
	r        *rand.Rand
}

func New() (*Handler, error) {
	out := Handler{
		r: rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	if err := out.parseConfig(); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	stickers, err := utils.ReadFile(out.cfg.FilePath)
	if err != nil {
		return nil, err
	}

	out.stickers = stickers

	return &out, nil
}

func (h *Handler) Handle(ctx telebot.Context) error {
	if h.r.Float32() > h.cfg.ReactChance {
		return nil
	}

	randomIndex := h.r.Intn(len(h.stickers))
	voice := h.stickers[randomIndex]

	return ctx.Reply(&telebot.Voice{File: telebot.File{FileID: voice}})
}
