package main

import (
	"context"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/marcboeker/go-duckdb/v2"
	"github.com/reijo1337/ToxicBot/internal/config"
	"github.com/reijo1337/ToxicBot/internal/features/stats"
	"github.com/reijo1337/ToxicBot/internal/handlers"
	"github.com/reijo1337/ToxicBot/internal/handlers/bulling"
	"github.com/reijo1337/ToxicBot/internal/handlers/on_sticker"
	"github.com/reijo1337/ToxicBot/internal/handlers/on_user_join"
	"github.com/reijo1337/ToxicBot/internal/handlers/on_user_left"
	"github.com/reijo1337/ToxicBot/internal/handlers/on_voice"
	"github.com/reijo1337/ToxicBot/internal/handlers/personal"
	"github.com/reijo1337/ToxicBot/internal/handlers/stat"
	"github.com/reijo1337/ToxicBot/internal/handlers/tagger"
	"github.com/reijo1337/ToxicBot/internal/infrastructure/ai/deepseek"
	"github.com/reijo1337/ToxicBot/internal/infrastructure/sheets"
	"github.com/reijo1337/ToxicBot/internal/infrastructure/sheets/google_spreadsheet"
	"github.com/reijo1337/ToxicBot/internal/infrastructure/storage/db"
	"github.com/reijo1337/ToxicBot/internal/message"
	"github.com/reijo1337/ToxicBot/internal/phrase_filter"
	"github.com/reijo1337/ToxicBot/pkg/logger"
	"github.com/reijo1337/ToxicBot/pkg/migrator"
	"gopkg.in/telebot.v3"
)

var AesKeyString string

func main() {
	logger := logger.New()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if AesKeyString == "" {
		logger.Fatal(ctx, "main.AesKeyString must be passed in -ldflags build flag")
	}

	random := rand.New(rand.NewSource(time.Now().UnixNano()))

	cfg, err := config.Parse()
	if err != nil {
		logger.Fatal(
			logger.WithError(ctx, err),
			"can't init config",
		)
	}

	if err := migrator.MigrateDB(cfg.DuckDbFilePath); err != nil {
		logger.Fatal(logger.WithError(ctx, err), "failed to migrate db")
	}

	dbpool, err := sqlx.Open("duckdb", cfg.DuckDbFilePath)
	if err != nil {
		logger.Fatal(
			logger.WithError(ctx, err),
			"failed to open database",
		)
	}

	if err := dbpool.Ping(); err != nil {
		logger.Fatal(
			logger.WithError(ctx, err),
			"failed to ping database",
		)
	}

	connGetter := db.NewConnGetter(dbpool)
	responseLogStorage := db.NewResponseLogStorage(connGetter)

	gs, err := google_spreadsheet.New(ctx)
	if err != nil {
		logger.Fatal(
			logger.WithError(ctx, err),
			"can't create google spreadsheet instance",
		)
	}

	sheetsRepository := sheets.New(gs)

	phraseFilter := phrase_filter.NewDefaultPhraseFilter()

	ai, err := deepseek.New()
	if err != nil {
		logger.Fatal(
			logger.WithError(ctx, err),
			"can't create deepseek client",
		)
	}

	stats, err := stats.New(AesKeyString, responseLogStorage, logger)
	if err != nil {
		logger.Fatal(
			logger.WithError(ctx, err),
			"can't init stats",
		)
	}

	generator, err := message.New(
		ctx,
		sheetsRepository,
		logger,
		random,
		phraseFilter,
		ai,
		cfg.BullingsUpdateMessagesPeriod,
		cfg.BullingsAIChance,
	)
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

	igorHandler, err := personal.New(
		ctx,
		"igor",
		sheetsRepository.GetPersonal("igor"),
		stats,
		750,
	)
	if err != nil {
		logger.Fatal(
			logger.WithError(ctx, err),
			"can't init personal igor handler",
		)
	}

	maxHandler, err := personal.New(
		ctx,
		"max",
		sheetsRepository.GetPersonal("max"),
		stats,
		200,
	)
	if err != nil {
		logger.Fatal(
			logger.WithError(ctx, err),
			"can't init personal max handler",
		)
	}

	kirillHandler, err := personal.New(
		ctx,
		"kirill",
		sheetsRepository.GetPersonal("kirill"),
		stats,
		150,
	)
	if err != nil {
		logger.Fatal(
			logger.WithError(ctx, err),
			"can't init personal kirill handler",
		)
	}

	bullingHandler, err := bulling.New(
		ctx,
		generator,
		stats,
		cfg.ThresholdCount,
		cfg.ThresholdTime,
		cfg.Cooldown,
	)
	if err != nil {
		logger.Fatal(
			logger.WithError(ctx, err),
			"can't init bulling handler",
		)
	}

	greetingsHandler, err := on_user_join.New(
		ctx,
		sheetsRepository,
		logger,
		random,
		stats,
		cfg.UpdateMessagesPeriod,
	)
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

	stickersReactionHandler, err := on_sticker.New(
		ctx,
		sheetsRepository,
		logger,
		random,
		stats,
		stickersFromPacks,
		cfg.StickerReactChance,
		cfg.UpdateStickersPeriod,
	)
	if err != nil {
		logger.Fatal(
			logger.WithError(ctx, err),
			"can't init sticker_reactions handler",
		)
	}

	onVoice, err := on_voice.New(
		ctx,
		sheetsRepository,
		logger,
		random,
		stats,
		cfg.VoiceReactChance,
		cfg.UpdateVoicesPeriod,
		b,
	)
	if err != nil {
		logger.Fatal(
			logger.WithError(ctx, err),
			"can't init on_voice handler",
		)
	}

	tagger, err := tagger.New(
		ctx,
		generator,
		sheetsRepository,
		b,
		logger,
		random,
		stats,
		cfg.TaggerIntervalFrom,
		cfg.TaggerIntervalTo,
		cfg.NicknamesUpdatePerios,
	)
	if err != nil {
		logger.Fatal(
			logger.WithError(ctx, err),
			"can't init tagger handler",
		)
	}

	onLeft := on_user_left.New(ctx, stats)

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

	b.Handle("/stat", stat.New(ctx, responseLogStorage).Handle)

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
