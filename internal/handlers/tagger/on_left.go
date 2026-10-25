package tagger

import (
	"errors"

	"gopkg.in/telebot.v3"
)

type OnLeftHandler struct {
	*Handler
}

func (h *Handler) OnLeft() *OnLeftHandler {
	return &OnLeftHandler{Handler: h}
}

func (h *OnLeftHandler) Slug() string {
	return "tagger_on_left"
}

func (h *OnLeftHandler) Handle(ctx telebot.Context) error {
	chat := ctx.Chat()
	sender := ctx.Sender()
	message := ctx.Message()

	if chat == nil || message == nil || message.UserLeft == nil {
		return nil
	}

	left := message.UserLeft

	// нас удалили из чата =( — инициатора обратно не добавляем
	if h.bot.Me.ID == left.ID {
		return h.forgetChat(h.ctx, chat.ID)
	}

	err := h.removeUser(h.ctx, chat.ID, left.ID)

	// кто-то удалил или сам?
	if sender != nil && sender.ID != left.ID {
		err = errors.Join(err, h.observe(h.ctx, chat, sender))
	}

	return err
}
