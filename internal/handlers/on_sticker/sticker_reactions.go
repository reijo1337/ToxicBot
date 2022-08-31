package on_sticker

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"math/rand"
	"sync"
	"time"

	"github.com/reijo1337/ToxicBot/internal/utils"
	"gopkg.in/telebot.v3"
)

type StickerReactions struct {
	cfg      config
	stickersFromFile []string
	stickers []string
	stickerPacks []string
	r        *rand.Rand
	mu sync.Mutex
}

func New(bot *telebot.Bot, stickerPacks []string, logger *logrus.Logger) (*StickerReactions, error) {
	out := StickerReactions{
		r: rand.New(rand.NewSource(time.Now().UnixNano())),
		stickerPacks: stickerPacks,
	}

	if err := out.parseConfig(); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	stickersFromFile, err := utils.ReadFile(out.cfg.FilePath)
	if err != nil {
		return nil, err
	}

	out.stickersFromFile = stickersFromFile

	err = out.UpdateStickersFromPacks(bot)
	if err != nil {
		logger.WithError(err).Warn("can't get stickers from sticker packs")

		out.mu.Lock()
		out.stickers = stickersFromFile
		out.mu.Unlock()
	}

	return &out, nil
}

func (sr *StickerReactions) UpdateStickersFromPacks(bot *telebot.Bot) error {
	stickersFromPacks, err := utils.GetStickersFromPacks(bot, sr.stickerPacks)
	if err != nil {
		return err
	}

	sr.mu.Lock()
	sr.stickers = append(sr.stickersFromFile, stickersFromPacks...)
	sr.mu.Unlock()

	return nil
}
func (sr *StickerReactions) Handle(ctx telebot.Context) error {
	if sr.r.Float32() > sr.cfg.ReactChance {
		return nil
	}

	sr.mu.Lock()
	randomIndex := sr.r.Intn(len(sr.stickers))
	sticker := sr.stickers[randomIndex]
	sr.mu.Unlock()

	return ctx.Reply(&telebot.Sticker{File: telebot.File{FileID: sticker}})
}
