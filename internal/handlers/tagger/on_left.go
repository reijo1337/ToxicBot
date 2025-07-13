package tagger

import (
	"fmt"

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

	// нас удалили из чата =(
	if h.bot.Me.ID == left.ID {
		h.mu.Lock()
		defer h.mu.Unlock()

		users := h.chatToUsers[chat.Recipient()]
		delete(h.chatToUsers, chat.Recipient())

		for _, userID := range users {
			key := fmt.Sprintf("%s:%d", chat.Recipient(), userID)
			delete(h.uniqueUsers, key)
		}

		return nil
	}

	// кого-то хлопнули - чистим инфу
	h.mu.Lock()
	users := h.chatToUsers[chat.Recipient()]

	if len(users) != 0 {
		i := 0
		for ; i < len(users); i++ {
			if users[i] == left.ID {
				break
			}
		}

		if i != len(users) {
			users[len(users)-1], users[i] = users[i], users[len(users)-1]
			h.chatToUsers[chat.Recipient()] = users[:len(users)-1]
		}

		key := fmt.Sprintf("%s:%d", chat.Recipient(), left.ID)
		delete(h.uniqueUsers, key)
	}
	h.mu.Unlock()

	// кто-то удалил или сам?
	if sender != nil && sender.ID != left.ID {
		h.addChatInfo(chat.Recipient(), sender)
	}

	return nil
}
