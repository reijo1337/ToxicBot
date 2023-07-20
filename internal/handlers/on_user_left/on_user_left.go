package on_user_left

import (
	"gopkg.in/telebot.v3"
)

const text = "Порвался"

type Handler struct{}

func New() Handler {
	return Handler{}
}

func (Handler) Slug() string {
	return "on_user_left"
}

func (Handler) Handle(ctx telebot.Context) error {
	return ctx.Reply(text)
}
