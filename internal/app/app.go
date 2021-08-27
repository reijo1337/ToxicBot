package app

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/sirupsen/logrus"
)

type Application struct {
	cfg      config
	bot      *tgbotapi.BotAPI
	logger   *logrus.Logger
	handlers []handler
	wg       sync.WaitGroup
}

type handler func(*tgbotapi.Message) (tgbotapi.Chattable, error)

func New(opts ...AppOption) (*Application, error) {
	a := &Application{}
	if err := a.parseConfig(); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	o := &options{}

	for _, opt := range opts {
		opt(o)
	}

	if o.logger == nil {
		o.logger = defaultLogger()
	}

	a.logger = o.logger

	bot, err := tgbotapi.NewBotAPI(a.cfg.TelegramToken)
	if err != nil {
		return nil, fmt.Errorf("init bot api: %w", err)
	}
	a.bot = bot

	return a, nil
}

func (a *Application) RegisterHandler(h handler) {
	a.handlers = append(a.handlers, h)
}

func (a *Application) Run() {
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := a.bot.GetUpdatesChan(u)
	if err != nil {
		a.logger.WithError(err).Fatal("get telegram updates")
	}

	a.logger.Infof("bot started as %s", a.bot.Self.UserName)

	for {
		select {
		case <-done:
			a.wg.Wait()
			return
		case update := <-updates:
			if update.Message == nil {
				break
			}

			for _, h := range a.handlers {
				a.wg.Add(1)
				go a.procHandler(h, update.Message)
			}
		}
	}
}

func (a *Application) procHandler(h handler, m *tgbotapi.Message) {
	defer a.wg.Done()

	response, err := h(m)
	if err != nil {
		a.logger.WithError(err).Errorf("process message [%d] from %s", m.MessageID, m.From.UserName)
	} else if response != nil {
		a.bot.Send(response)
	}
}
