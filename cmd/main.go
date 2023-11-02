package main

import (
	"context"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/reijo1337/ToxicBot/internal/config"
	"github.com/reijo1337/ToxicBot/internal/handlers"
	"github.com/reijo1337/ToxicBot/internal/handlers/bulling"
	"github.com/reijo1337/ToxicBot/internal/handlers/on_sticker"
	"github.com/reijo1337/ToxicBot/internal/handlers/on_user_join"
	"github.com/reijo1337/ToxicBot/internal/handlers/on_user_left"
	"github.com/reijo1337/ToxicBot/internal/handlers/on_voice"
	"github.com/reijo1337/ToxicBot/internal/handlers/personal"
	"github.com/reijo1337/ToxicBot/internal/handlers/tagger"
	"github.com/reijo1337/ToxicBot/internal/infrastructure/sheets"
	"github.com/reijo1337/ToxicBot/internal/infrastructure/sheets/google_spreadsheet"
	"github.com/reijo1337/ToxicBot/internal/message"
	"github.com/reijo1337/ToxicBot/pkg/logger"
	"gopkg.in/telebot.v3"
)

func main() {
	logger := logger.New()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	random := rand.New(rand.NewSource(time.Now().UnixNano()))

	cfg, err := config.Parse()
	if err != nil {
		logger.Fatal(
			logger.WithError(ctx, err),
			"can't init config",
		)
	}

	gs, err := google_spreadsheet.New(ctx)
	if err != nil {
		logger.Fatal(
			logger.WithError(ctx, err),
			"can't create google spreadsheet instance",
		)
	}

	sheetsRepository := sheets.New(gs)

	generator, err := message.New(ctx, sheetsRepository, logger, random, cfg.BullingsUpdateMessagesPeriod, cfg.BullingsMarkovChance)
	if err != nil {
		logger.Fatal(
			logger.WithError(ctx, err),
			"can't create random text generator",
		)
	}

	pref := telebot.Settings{
		Token:  cfg.TelegramToken,
		Poller: &telebot.LongPoller{Timeout: cfg.TelegramLongPollTimeout},
		OnError: func(err error, ctx telebot.Context) {
			logger.Error(
				logger.WithField(
					logger.WithError(context.Background(), err),
					"update", ctx.Update(),
				),
				"can't handle update",
			)
		},
	}

	b, err := telebot.NewBot(pref)
	if err != nil {
		logger.Fatal(
			logger.WithError(ctx, err),
			"can't init bot api",
		)
	}

	igorHandler, err := personal.New("igor", sheetsRepository.GetPersonal("igor"), 750)
	if err != nil {
		logger.Fatal(
			logger.WithError(ctx, err),
			"can't init personal igor handler",
		)
	}

	maxHandler, err := personal.New("max", sheetsRepository.GetPersonal("max"), 200)
	if err != nil {
		logger.Fatal(
			logger.WithError(ctx, err),
			"can't init personal max handler",
		)
	}

	kirillHandler, err := personal.New("kirill", sheetsRepository.GetPersonal("kirill"), 150)
	if err != nil {
		logger.Fatal(
			logger.WithError(ctx, err),
			"can't init personal kirill handler",
		)
	}

	bullingHandler, err := bulling.New(ctx, generator, cfg.ThresholdCount, cfg.ThresholdTime, cfg.Cooldown)
	if err != nil {
		logger.Fatal(
			logger.WithError(ctx, err),
			"can't init bulling handler",
		)
	}

	greetingsHandler, err := on_user_join.New(ctx, sheetsRepository, logger, random, cfg.UpdateMessagesPeriod)
	if err != nil {
		logger.Fatal(
			logger.WithError(ctx, err),
			"can't init greetings handler",
		)
	}

	stickersFromPacks := []string{}
	if len(cfg.StickerSets) > 0 {
		stickersFromPacks, err = getStickersFromPacks(b, cfg.StickerSets)
		if err != nil {
			logger.Warn(
				logger.WithError(ctx, err),
				"can't get stickers from sticker packs",
			)
		}
	}

	stickersReactionHandler, err := on_sticker.New(ctx, sheetsRepository, logger, random, stickersFromPacks, cfg.StickerReactChance, cfg.UpdateStickersPeriod)
	if err != nil {
		logger.Fatal(
			logger.WithError(ctx, err),
			"can't init sticker_reactions handler",
		)
	}

	onVoice, err := on_voice.New(ctx, sheetsRepository, logger, random, cfg.VoiceReactChance, cfg.UpdateVoicesPeriod)
	if err != nil {
		logger.Fatal(
			logger.WithError(ctx, err),
			"can't init on_voice handler",
		)
	}

	tagger, err := tagger.New(ctx, generator, sheetsRepository, b, logger, random, cfg.TaggerIntervalFrom, cfg.TaggerIntervalTo, cfg.NicknamesUpdatePerios)
	if err != nil {
		logger.Fatal(
			logger.WithError(ctx, err),
			"can't init tagger handler",
		)
	}

	onLeft := on_user_left.New()

	b.Handle(
		telebot.OnText,
		handlers.New(
			telebot.OnText,
			igorHandler,
			maxHandler,
			bullingHandler,
			tagger,
			kirillHandler,
		).Handle,
	)

	b.Handle(
		telebot.OnSticker,
		handlers.New(
			telebot.OnSticker,
			stickersReactionHandler,
			igorHandler,
			maxHandler,
			bullingHandler,
			tagger,
			kirillHandler,
		).Handle,
	)

	b.Handle(
		telebot.OnVoice,
		handlers.New(
			telebot.OnVoice,
			onVoice,
			igorHandler,
			maxHandler,
			tagger,
			kirillHandler,
		).Handle,
	)

	b.Handle(
		telebot.OnUserJoined,
		handlers.New(
			telebot.OnUserJoined,
			greetingsHandler,
			tagger.OnJoin(),
		).Handle,
	)

	b.Handle(
		telebot.OnUserLeft,
		handlers.New(
			telebot.OnUserLeft,
			onLeft,
			tagger.OnLeft(),
		).Handle,
	)

	b.Handle(telebot.OnMedia, tagger.Handle)

	go func() {
		logger.Info(
			logger.WithField(ctx, "user_name", b.Me.Username),
			"bot started",
		)
		b.Start()
	}()

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	<-done
	b.Stop()
}

func getStickersFromPacks(bot *telebot.Bot, stickerPacksNames []string) ([]string, error) {

	var stickers []string

	for _, pack := range stickerPacksNames {
		stickerPack, err := bot.StickerSet(pack)
		if err != nil {
			return nil, err
		}

		for _, sticker := range stickerPack.Stickers {
			if sticker.FileID != "" {
				stickers = append(stickers, sticker.FileID)
			}
		}
	}

	return stickers, nil
}
