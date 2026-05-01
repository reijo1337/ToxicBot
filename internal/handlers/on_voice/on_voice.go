package on_voice

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

type Handler struct {
	ctx              context.Context
	storage          voicesRepository
	r                randomizer
	downloader       downloader
	logger           logger
	statIncer        statIncer
	settingsProvider settingsProvider
	history          historyBuffer
	replier          botReplier
	botAuthor        string
	voices           []string
	muVcs            sync.RWMutex
	updatePeriod     time.Duration
}

func New(
	ctx context.Context,
	stor voicesRepository,
	logger logger,
	r randomizer,
	statIncer statIncer,
	settingsProvider settingsProvider,
	history historyBuffer,
	replier botReplier,
	updatePeriod time.Duration,
	downloader downloader,
	botAuthor string,
) (*Handler, error) {
	out := Handler{
		ctx:              ctx,
		storage:          stor,
		logger:           logger,
		r:                r,
		statIncer:        statIncer,
		settingsProvider: settingsProvider,
		history:          history,
		replier:          replier,
		updatePeriod:     updatePeriod,
		downloader:       downloader,
		botAuthor:        botAuthor,
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
	chat := pointer.From(ctx.Chat())
	sender := pointer.From(ctx.Sender())
	msg := ctx.Message()

	author := message.SanitizeAuthor(sender.Username, sender.FirstName, sender.ID, sender.IsBot)
	replyToID := 0
	if msg.ReplyTo != nil {
		replyToID = msg.ReplyTo.ID
	}
	h.history.Add(chat.ID, chathistory.Entry{
		ID:        msg.ID,
		Time:      msg.Time(),
		Author:    author,
		Text:      "*прислал голосовое*",
		ReplyToID: replyToID,
		FromBot:   false,
	})

	settings, err := h.settingsProvider.GetForChat(h.ctx, chat.ID)
	if err != nil {
		return fmt.Errorf("can't get chat settings: %w", err)
	}

	if h.r.Float32() > settings.VoiceReactChance {
		return nil
	}

	h.muVcs.RLock()
	defer h.muVcs.RUnlock()

	go h.statIncer.Inc(
		h.ctx,
		chat.ID,
		sender.ID,
		stats.OnVoiceOperationType,
	)

	randomIndex := h.r.Intn(len(h.voices))
	voiceID := h.voices[randomIndex]

	voice, err := h.downloader.FileByID(voiceID)
	if err != nil {
		return fmt.Errorf("can't get voice %s: %w", voiceID, err)
	}

	if err := ctx.Notify(telebot.RecordingAudio); err != nil {
		return fmt.Errorf("ctx.Notify error: %w", err)
	}
	time.Sleep(time.Duration(h.r.Intn(15) * 1_000_000_000))

	response := telebot.Voice{File: voice}

	sent, err := h.replier.Reply(msg, &response)
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
		return err
	}

	h.history.Add(chat.ID, chathistory.Entry{
		ID:        sent.ID,
		Time:      time.Now(),
		Author:    h.botAuthor,
		Text:      "*прислал голосовое*",
		ReplyToID: msg.ID,
		FromBot:   true,
	})

	return nil
}
