package tagger

import (
	"errors"
	"slices"

	"gopkg.in/telebot.v3"
)

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

	joined := joinedUsers(ctx.Message())
	if chat == nil || len(joined) == 0 {
		return nil
	}

	var err error
	for i := range joined {
		// Событие само подтверждает вступление — запрос к Telegram не нужен.
		err = errors.Join(err, h.addUser(h.ctx, chat.ID, &joined[i]))
	}

	// Добавивший участников отличается от вступивших — его членство проверяем отдельно.
	if sender != nil && !slices.ContainsFunc(joined, func(u telebot.User) bool {
		return u.ID == sender.ID
	}) {
		err = errors.Join(err, h.observe(h.ctx, chat, sender))
	}

	return err
}

// joinedUsers возвращает всех вступивших. При вступлении нескольких пользователей
// одним сообщением telebot вызывает обработчик для каждого, подменяя UserJoined в общем
// сообщении, а обработчики работают параллельно. Поэтому UserJoined читается, только
// когда UsersJoined пуст.
func joinedUsers(msg *telebot.Message) []telebot.User {
	switch {
	case msg == nil:
		return nil
	case len(msg.UsersJoined) > 0:
		return msg.UsersJoined
	case msg.UserJoined != nil:
		return []telebot.User{*msg.UserJoined}
	default:
		return nil
	}
}
