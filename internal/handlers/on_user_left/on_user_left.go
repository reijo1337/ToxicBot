package on_user_left

import (
	"context"

	"github.com/reijo1337/ToxicBot/internal/features/stats"
	"github.com/reijo1337/ToxicBot/pkg/pointer"
	"gopkg.in/telebot.v3"
)

type statIncer interface {
	Inc(ctx context.Context, chatID, userID int64, op stats.OperationType, opts ...stats.Option)
}

const text = "Порвался"

type Handler struct {
	ctx       context.Context
	statIncer statIncer
}

func New(ctx context.Context, statIncer statIncer) *Handler {
	return &Handler{
		ctx:       ctx,
		statIncer: statIncer,
	}
}

func (h *Handler) Slug() string {
	return "on_user_left"
}

func (h *Handler) Handle(ctx telebot.Context) error {
	go h.statIncer.Inc(
		h.ctx,
		pointer.From(ctx.Chat()).ID,
		pointer.From(ctx.Sender()).ID,
		stats.OnUserLeftOperationType,
	)

	return ctx.Reply(text)
}
