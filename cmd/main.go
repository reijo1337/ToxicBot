package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/reijo1337/ToxicBot/internal/handlers/on_sticker"
	"github.com/reijo1337/ToxicBot/internal/handlers/on_text"
	"github.com/reijo1337/ToxicBot/internal/handlers/on_text/bulling"
	"github.com/reijo1337/ToxicBot/internal/handlers/on_text/igor"
	"github.com/reijo1337/ToxicBot/internal/handlers/on_user_join"
	"github.com/reijo1337/ToxicBot/internal/handlers/on_user_left"
	"github.com/reijo1337/ToxicBot/internal/handlers/on_voice"
	"gopkg.in/telebot.v3"

	"github.com/sirupsen/logrus"
)

type config struct {
	TelegramToken           string        `envconfig:"TELEGRAM_TOKEN" required:"true"`
	TelegramLongPollTimeout time.Duration `envconfig:"TELEGRAM_LONG_POLL_TIMEOUT" default:"10s"`
	StickerSets             []string      `envconfig:"STICKER_SETS" default:"static_bulling_by_stickersthiefbot"`
	StickersUpdateInterval	time.Duration `envconfig:"STICKERS_UPDATE_INTERVAL" default:"600s"`
}

func main() {
	logger := newLogger()

	cfg, err := newConfig()
	if err != nil {
		logger.WithError(err).Fatal("can't init config")
	}

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

	b, err := telebot.NewBot(pref)
	if err != nil {
		logger.WithError(err).Fatal("can't init bot api")
	}

	igorHandler, err := igor.New()
	if err != nil {
		logger.WithError(err).Fatal("init on_text igor handler")
	}

	bullingHandler, err := bulling.New()
	if err != nil {
		logger.WithError(err).Fatal("init on_text bulling handler")
	}

	b.Handle(
		telebot.OnText,
		on_text.New(
			igorHandler,
			bullingHandler,
		).Handle,
	)

	greetingsHandler, err := on_user_join.New()
	if err != nil {
		logger.WithError(err).Fatal("can't init on_user_join handler")
	}
	b.Handle(telebot.OnUserJoined, greetingsHandler.Handle)

	b.Handle(telebot.OnUserLeft, on_user_left.Handle)

	stickersReactionHandler, err := on_sticker.New(b, cfg.StickerSets, logger)
	if err != nil {
		logger.WithError(err).Fatal("can't init on_sticker handler")
	}

	b.Handle(telebot.OnSticker, stickersReactionHandler.Handle)

	onVoice, err := on_voice.New()
	if err != nil {
		logger.WithError(err).Fatal("can't init on_voice handler")
	}
	b.Handle(telebot.OnVoice, onVoice.Handle)

	go func() {
		logger.WithField("user_name", b.Me.Username).Info("bot started")
		b.Start()
	}()

	stickersUpdateDone := make(chan struct{})
	ticker := time.NewTicker(cfg.StickersUpdateInterval)
	go func() {
		for {
			select {
			case <-stickersUpdateDone:
				return
			case <-ticker.C:
				err = stickersReactionHandler.UpdateStickersFromPacks(b)
				if err != nil {
					logger.WithError(err).Warn("can't get stickers from sticker packs")
				}
			}
		}
	}()

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	<-done
	stickersUpdateDone <- struct{}{}
	ticker.Stop()
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
		envconfig.Usage("", cfg)
		return nil, err
	}

	return &cfg, nil
}
