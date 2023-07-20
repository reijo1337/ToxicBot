package tagger

import "gopkg.in/telebot.v3"

type OnJoinHandler struct {
	*Handler
}

func (h *Handler) OnJoin() *OnJoinHandler {
	return &OnJoinHandler{Handler: h}
}

func (h *OnJoinHandler) Slug() string {
	return "tagger_on_join"
}

func (h *OnJoinHandler) Handle(ctx telebot.Context) error {
	chat := ctx.Chat()
	sender := ctx.Sender()
	message := ctx.Message()

	if chat == nil || message == nil || message.UserJoined == nil {
		return nil
	}

	h.addChatInfo(chat.Recipient(), message.UserJoined)

	if sender != nil {
		h.addChatInfo(chat.Recipient(), sender)
	}

	return nil
}
