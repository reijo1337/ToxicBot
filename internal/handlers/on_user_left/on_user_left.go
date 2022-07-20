package on_user_left

import (
	"gopkg.in/telebot.v3"
)

const text = "Порвался"

func Handle(ctx telebot.Context) error {
	return ctx.Reply(text)
}
