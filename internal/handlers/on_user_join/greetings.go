package on_user_join

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gopkg.in/telebot.v3"
)

type Greetings struct {
	storage              greetingsRepository
	logger               logger
	r                    randomizer
	messages             []string
	muMsg                sync.RWMutex
	updateMessagesPeriod time.Duration
}

func New(
	ctx context.Context,
	stor greetingsRepository,
	logger logger,
	r randomizer,
	updateMessagesPeriod time.Duration,
) (*Greetings, error) {
	out := Greetings{
		storage:              stor,
		logger:               logger,
		r:                    r,
		updateMessagesPeriod: updateMessagesPeriod,
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

func (g *Greetings) Slug() string {
	return "greetings"
}
