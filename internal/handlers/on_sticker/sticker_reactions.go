package on_sticker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/reijo1337/ToxicBot/internal/chatsettings"
	"github.com/reijo1337/ToxicBot/internal/features/stats"
	"github.com/reijo1337/ToxicBot/pkg/pointer"
	"gopkg.in/telebot.v3"
)

type settingsProvider interface {
	GetForChat(ctx context.Context, chatID int64) (*chatsettings.Settings, error)
}

type StickerReactions struct {
	ctx                  context.Context
	storage              stickerRepository
	r                    randomizer
	logger               logger
	statIncer            statIncer
	settingsProvider     settingsProvider
	stickers             []string
	stickersFromPacks    []string
	muStk                sync.RWMutex
	updateStickersPeriod time.Duration
}

func New(
	ctx context.Context,
	stor stickerRepository,
	logger logger,
	r randomizer,
	statIncer statIncer,
	stickersFromPacks []string,
	settingsProvider settingsProvider,
	updateStickersPeriod time.Duration,
) (*StickerReactions, error) {
	out := StickerReactions{
		ctx:                  ctx,
		storage:              stor,
		logger:               logger,
		stickersFromPacks:    stickersFromPacks,
		r:                    r,
		statIncer:            statIncer,
		settingsProvider:     settingsProvider,
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
	chat := pointer.From(ctx.Chat())

	settings, err := sr.settingsProvider.GetForChat(sr.ctx, chat.ID)
	if err != nil {
		return fmt.Errorf("can't get chat settings: %w", err)
	}

	if sr.r.Float32() > settings.StickerReactChance {
		return nil
	}

	sr.muStk.RLock()
	s := make([]string, 0, len(sr.stickersFromPacks)+len(sr.stickers))
	s = append(s, sr.stickers...)
	s = append(s, sr.stickersFromPacks...)
	sr.muStk.RUnlock()

	randomIndex := sr.r.Intn(len(s))
	sticker := s[randomIndex]

	go sr.statIncer.Inc(
		sr.ctx,
		chat.ID,
		pointer.From(ctx.Sender()).ID,
		stats.OnStickerOperationType,
	)

	return ctx.Reply(&telebot.Sticker{File: telebot.File{FileID: sticker}})
}
