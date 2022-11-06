package on_user_join

import (
	"context"
	"fmt"
	"github.com/reijo1337/ToxicBot/internal/storage"
	"github.com/sirupsen/logrus"
	"math/rand"
	"sync"
	"time"

	"gopkg.in/telebot.v3"
)

type Greetings struct {
	cfg config

	storage storage.Manager
	logger  *logrus.Logger

	messages []string
	muMsg    sync.RWMutex

	r *rand.Rand
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
	ctx.Reply(text)

	return nil
}
