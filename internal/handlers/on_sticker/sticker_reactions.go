package on_sticker

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

type StickerReactions struct {
	storage           storage.Manager
	r                 *rand.Rand
	logger            *logrus.Logger
	stickers          []string
	stickersFromPacks []string
	cfg               config
	muStk             sync.RWMutex
}

func New(ctx context.Context, stor storage.Manager, logger *logrus.Logger, stickersFromPacks []string) (*StickerReactions, error) {
	out := StickerReactions{
		storage:           stor,
		logger:            logger,
		stickersFromPacks: stickersFromPacks,
		r:                 rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	if err := out.parseConfig(); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := out.reloadStickers(); err != nil {
		return nil, fmt.Errorf("cannot reload stickers: %w", err)
	}
	go out.runUpdater(ctx)

	return &out, nil
}

func (sr *StickerReactions) Handle(ctx telebot.Context) error {
	if sr.r.Float32() > sr.cfg.ReactChance {
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
