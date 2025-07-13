package on_sticker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gopkg.in/telebot.v3"
)

type StickerReactions struct {
	storage              stickerRepository
	r                    randomizer
	logger               logger
	stickers             []string
	stickersFromPacks    []string
	muStk                sync.RWMutex
	reactChance          float32
	updateStickersPeriod time.Duration
}

func New(
	ctx context.Context,
	stor stickerRepository,
	logger logger,
	r randomizer,
	stickersFromPacks []string,
	reactChance float32,
	updateStickersPeriod time.Duration,
) (*StickerReactions, error) {
	out := StickerReactions{
		storage:              stor,
		logger:               logger,
		stickersFromPacks:    stickersFromPacks,
		r:                    r,
		reactChance:          reactChance,
		updateStickersPeriod: updateStickersPeriod,
	}

	if err := out.reloadStickers(); err != nil {
		return nil, fmt.Errorf("cannot reload stickers: %w", err)
	}

	go out.runUpdater(ctx)

	return &out, nil
}

func (*StickerReactions) Slug() string {
	return "sticker_reactions"
}

func (sr *StickerReactions) Handle(ctx telebot.Context) error {
	if sr.r.Float32() > sr.reactChance {
		return nil
	}

	sr.muStk.RLock()
	s := make([]string, 0, len(sr.stickersFromPacks)+len(sr.stickers))
	s = append(s, sr.stickers...)
	s = append(s, sr.stickersFromPacks...)
	sr.muStk.RUnlock()

	randomIndex := sr.r.Intn(len(s))
	sticker := s[randomIndex]

	return ctx.Reply(&telebot.Sticker{File: telebot.File{FileID: sticker}})
}
