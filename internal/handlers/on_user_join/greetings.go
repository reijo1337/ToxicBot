package on_user_join

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

type Greetings struct {
	storage  storage.Manager
	logger   *logrus.Logger
	r        *rand.Rand
	messages []string
	cfg      config
	muMsg    sync.RWMutex
}

func New(ctx context.Context, stor storage.Manager, logger *logrus.Logger) (*Greetings, error) {
	out := Greetings{
		storage: stor,
		logger:  logger,
		r:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	if err := out.parseConfig(); err != nil {
		return nil, fmt.Errorf("cannot parse config: %w", err)
	}

	if err := out.reloadMessages(); err != nil {
		return nil, fmt.Errorf("cannot load messages: %w", err)
	}

	go out.runUpdater(ctx)

	return &out, nil
}

func (g *Greetings) Handle(ctx telebot.Context) error {
	randomIndex := g.r.Intn(len(g.messages))
	text := g.messages[randomIndex]

	return ctx.Reply(text)
}
