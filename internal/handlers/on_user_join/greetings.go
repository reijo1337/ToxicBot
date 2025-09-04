package on_user_join

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/reijo1337/ToxicBot/internal/features/stats"
	"github.com/reijo1337/ToxicBot/pkg/pointer"
	"gopkg.in/telebot.v3"
)

type Greetings struct {
	ctx                  context.Context
	storage              greetingsRepository
	logger               logger
	r                    randomizer
	statIncer            statIncer
	messages             []string
	muMsg                sync.RWMutex
	updateMessagesPeriod time.Duration
}

func New(
	ctx context.Context,
	stor greetingsRepository,
	logger logger,
	r randomizer,
	statIncer statIncer,
	updateMessagesPeriod time.Duration,
) (*Greetings, error) {
	out := Greetings{
		ctx:                  ctx,
		storage:              stor,
		logger:               logger,
		r:                    r,
		statIncer:            statIncer,
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

	go g.statIncer.Inc(
		g.ctx,
		pointer.From(ctx.Chat()).ID,
		pointer.From(ctx.Sender()).ID,
		stats.OnUserJoinOperationType,
	)

	return ctx.Reply(text)
}

func (g *Greetings) Slug() string {
	return "greetings"
}
