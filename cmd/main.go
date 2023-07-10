package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/reijo1337/ToxicBot/internal/google_spreadsheet"
	"github.com/reijo1337/ToxicBot/internal/handlers/on_sticker"
	"github.com/reijo1337/ToxicBot/internal/handlers/on_text"
	"github.com/reijo1337/ToxicBot/internal/handlers/on_text/bulling"
	"github.com/reijo1337/ToxicBot/internal/handlers/on_text/igor"
	"github.com/reijo1337/ToxicBot/internal/handlers/on_text/max"
	"github.com/reijo1337/ToxicBot/internal/handlers/on_user_join"
	"github.com/reijo1337/ToxicBot/internal/handlers/on_user_left"
	"github.com/reijo1337/ToxicBot/internal/handlers/on_voice"
	"github.com/reijo1337/ToxicBot/internal/storage"
	"github.com/reijo1337/ToxicBot/internal/utils"
	"github.com/sirupsen/logrus"
	"gopkg.in/telebot.v3"
)

type config struct {
	TelegramToken           string        `envconfig:"TELEGRAM_TOKEN" required:"true"`
	StickerSets             []string      `envconfig:"STICKER_SETS" default:"static_bulling_by_stickersthiefbot"`
	TelegramLongPollTimeout time.Duration `envconfig:"TELEGRAM_LONG_POLL_TIMEOUT" default:"10s"`
}

func main() {
	var err error
	logger := newLogger()
	defer func() {
		if err != nil {
			logger.WithError(err).Fatal("application close with error")
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var cfg *config
	cfg, err = newConfig()
	if err != nil {
		err = fmt.Errorf("cannot initialize config: %w", err)
		return
	}

	var gs google_spreadsheet.Manager
	gs, err = google_spreadsheet.New(ctx)
	if err != nil {
		err = fmt.Errorf("can't create google spreadsheet instance: %w", err)
		return
	}

	stor := storage.New(gs)

	pref := telebot.Settings{
		Token:  cfg.TelegramToken,
		Poller: &telebot.LongPoller{Timeout: cfg.TelegramLongPollTimeout},
		OnError: func(err error, ctx telebot.Context) {
			logger.
				WithError(err).
				WithField("update", ctx.Update()).
				Error("can't handle update")
		},
	}

	var b *telebot.Bot
	b, err = telebot.NewBot(pref)
	if err != nil {
		err = fmt.Errorf("can't init bot api: %w", err)
		return
	}

	var igorHandler on_text.SubHandler
	igorHandler, err = igor.New(stor)
	if err != nil {
		err = fmt.Errorf("init on_text igor handler: %w", err)
		return
	}

	var maxHandler on_text.SubHandler
	maxHandler, err = max.New(stor)
	if err != nil {
		err = fmt.Errorf("init on_text max handler: %w", err)
		return
	}

	var bullingHandler on_text.SubHandler
	bullingHandler, err = bulling.New(ctx, stor, logger)
	if err != nil {
		err = fmt.Errorf("init on_text bulling handler: %w", err)
		return
	}

	b.Handle(
		telebot.OnText,
		on_text.New(
			igorHandler,
			maxHandler,
			bullingHandler,
		).Handle,
	)

	var greetingsHandler *on_user_join.Greetings
	greetingsHandler, err = on_user_join.New(ctx, stor, logger)
	if err != nil {
		err = fmt.Errorf("can't init on_user_join handler: %w", err)
		return
	}
	b.Handle(telebot.OnUserJoined, greetingsHandler.Handle)

	b.Handle(telebot.OnUserLeft, on_user_left.Handle)

	stickersFromPacks := []string{}
	if len(cfg.StickerSets) > 0 {
		stickersFromPacks, err = utils.GetStickersFromPacks(b, cfg.StickerSets)
		if err != nil {
			logger.WithError(err).Warn("can't get stickers from sticker packs")
		}
	}

	var stickersReactionHandler *on_sticker.StickerReactions
	stickersReactionHandler, err = on_sticker.New(ctx, stor, logger, stickersFromPacks)
	if err != nil {
		err = fmt.Errorf("can't init on_sticker handler: %w", err)
		return
	}

	b.Handle(telebot.OnSticker, stickersReactionHandler.Handle)

	var onVoice *on_voice.Handler
	onVoice, err = on_voice.New(ctx, stor, logger)
	if err != nil {
		err = fmt.Errorf("can't init on_voice handler: %w", err)
		return
	}
	b.Handle(telebot.OnVoice, onVoice.Handle)

	go func() {
		logger.WithField("user_name", b.Me.Username).Info("bot started")
		b.Start()
	}()

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	<-done
	b.Stop()
}

func newLogger() *logrus.Logger {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)
	logger.SetFormatter(&logrus.TextFormatter{})
	logger.SetReportCaller(true)

	return logger
}

func newConfig() (*config, error) {
	var cfg config
	if err := envconfig.Process("", &cfg); err != nil {
		if err = envconfig.Usage("", cfg); err != nil {
			return nil, err
		}
		return nil, err
	}

	return &cfg, nil
}
