package on_sticker

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/reijo1337/ToxicBot/internal/utils"
	"gopkg.in/telebot.v3"
)

type StickerReactions struct {
	cfg      config
	stickers []string
	r        *rand.Rand
}

func New() (*StickerReactions, error) {
	out := StickerReactions{
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

func (sr *StickerReactions) Handle(ctx telebot.Context) error {
	if sr.r.Float32() > sr.cfg.ReactChance {
		return nil
	}

	randomIndex := sr.r.Intn(len(sr.stickers))
	sticker := sr.stickers[randomIndex]

	return ctx.Reply(&telebot.Sticker{File: telebot.File{FileID: sticker}})
}
