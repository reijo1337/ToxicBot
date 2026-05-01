package on_sticker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/reijo1337/ToxicBot/internal/features/chatsettings"
	"github.com/reijo1337/ToxicBot/internal/features/message"
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
	history              historyBuffer
	replier              botReplier
	botAuthor            string
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
	history historyBuffer,
	replier botReplier,
	updateStickersPeriod time.Duration,
	botAuthor string,
) (*StickerReactions, error) {
	out := StickerReactions{
		ctx:                  ctx,
		storage:              stor,
		logger:               logger,
		stickersFromPacks:    stickersFromPacks,
		r:                    r,
		statIncer:            statIncer,
		settingsProvider:     settingsProvider,
		history:              history,
		replier:              replier,
		updateStickersPeriod: updateStickersPeriod,
		botAuthor:            botAuthor,
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
	sender := pointer.From(ctx.Sender())
	msg := ctx.Message()

	author := message.SanitizeAuthor(sender.Username, sender.FirstName, sender.ID, sender.IsBot)
	replyToID := 0
	if msg.ReplyTo != nil {
		replyToID = msg.ReplyTo.ID
	}
	sr.history.Add(chat.ID, chathistory.Entry{
		ID:        msg.ID,
		Time:      msg.Time(),
		Author:    author,
		Text:      "*прислал стикер*",
		ReplyToID: replyToID,
		FromBot:   false,
	})

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
		sender.ID,
		stats.OnStickerOperationType,
	)

	sent, err := sr.replier.Reply(msg, &telebot.Sticker{File: telebot.File{FileID: sticker}})
	if err != nil {
		return err
	}

	sr.history.Add(chat.ID, chathistory.Entry{
		ID:        sent.ID,
		Time:      time.Now(),
		Author:    sr.botAuthor,
		Text:      "*прислал стикер*",
		ReplyToID: msg.ID,
		FromBot:   true,
	})

	return nil
}
