package personal

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/reijo1337/ToxicBot/internal/features/stats"
	"github.com/reijo1337/ToxicBot/pkg/pointer"
	"gopkg.in/telebot.v3"
)

const (
	envFormat  = "%s_ID"
	slugFormat = "personal_%s"
)

type Handler struct {
	ctx        context.Context
	repository messageRepository
	statIncer  statIncer
	random     *rand.Rand
	slug       string
	id         int64
	chance     int
}

func New(
	ctx context.Context,
	name string,
	repository messageRepository,
	statIncer statIncer,
	chance int,
) (*Handler, error) {
	env := fmt.Sprintf(envFormat, strings.ToUpper(name))
	id, err := strconv.ParseInt(os.Getenv(env), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse %s id from env: %w", name, err)
	}

	return &Handler{
		ctx:        ctx,
		slug:       fmt.Sprintf(slugFormat, strings.ToLower(name)),
		id:         id,
		random:     rand.New(rand.NewSource(time.Now().UnixNano())),
		repository: repository,
		statIncer:  statIncer,
		chance:     chance,
	}, nil
}

func (h *Handler) Slug() string {
	return h.slug
}

func (i *Handler) Handle(ctx telebot.Context) error {
	user := ctx.Sender()
	if user == nil || user.ID != i.id {
		return nil
	}

	if i.random.Intn(i.chance) != 0 {
		return nil
	}

	messages, err := i.repository.GetEnabledMessages()
	if err != nil {
		return fmt.Errorf("can't get messages from repositorey: %w", err)
	}

	if idx := i.random.Intn(len(messages)); idx == 0 {
		go i.statIncer.Inc(
			i.ctx,
			pointer.From(ctx.Chat()).ID,
			pointer.From(ctx.Sender()).ID,
			stats.PersonalOperationType,
		)

		return ctx.Reply(messages[idx])
	}

	return nil
}
