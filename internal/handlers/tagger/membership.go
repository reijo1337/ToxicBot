package tagger

import (
	"context"
	"fmt"

	"github.com/reijo1337/ToxicBot/internal/domain/chat"
	"github.com/reijo1337/ToxicBot/internal/features/message"
	"gopkg.in/telebot.v3"
)

type membership int

const (
	membershipUnknown membership = iota
	membershipPresent
	membershipGone
)

// chatEpoch считает известия о составе чата: present — каждое подтверждение присутствия,
// removed — каждое известие о выходе, в том числе неизвестного участника. Ответ
// getChatMember устарел, если за время запроса пришло противоположное известие.
type chatEpoch struct {
	present uint64
	removed uint64
}

func (e chatEpoch) outdates(since chatEpoch, state membership) bool {
	if state == membershipPresent {
		return e.removed != since.removed
	}
	return e.present != since.present
}

// membershipOf переводит ответ getChatMember в решение о кандидате. Неоднозначный
// ответ не считается выходом.
func membershipOf(member *telebot.ChatMember) membership {
	if member == nil {
		return membershipUnknown
	}

	switch member.Role {
	case telebot.Creator, telebot.Administrator, telebot.Member:
		return membershipPresent
	case telebot.Restricted:
		if member.Member {
			return membershipPresent
		}
		return membershipGone
	case telebot.Left, telebot.Kicked:
		return membershipGone
	default:
		return membershipUnknown
	}
}

// scheduleKnownChats назначает каждому непустому чату из реестра новый случайный интервал.
func (h *Handler) scheduleKnownChats() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, chatID := range h.users.Chats() {
		h.scheduleLocked(chatID)
	}
}

// observe учитывает автора сообщения или инициатора события после проверки членства.
func (h *Handler) observe(ctx context.Context, tgChat *telebot.Chat, user *telebot.User) error {
	if user.IsBot {
		return nil
	}

	epoch := h.epochOf(tgChat.ID)

	member, err := h.bot.ChatMemberOf(tgChat, user)
	if err != nil {
		return fmt.Errorf("can't check if user is member of chat: %w", err)
	}

	state := membershipOf(member)
	if state == membershipUnknown {
		return nil
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Ответ устарел, пока шёл запрос. Следующее сообщение участника проверит его заново.
	if h.epochs[tgChat.ID].outdates(epoch, state) {
		return nil
	}

	if state == membershipPresent {
		return h.addUserLocked(ctx, tgChat.ID, user)
	}
	return h.removeUserLocked(ctx, tgChat.ID, user.ID)
}

func (h *Handler) epochOf(chatID int64) chatEpoch {
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.epochs[chatID]
}

// addUser добавляет участника в реестр и назначает чату задачу, если её нет.
func (h *Handler) addUser(ctx context.Context, chatID int64, user *telebot.User) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.addUserLocked(ctx, chatID, user)
}

func (h *Handler) addUserLocked(ctx context.Context, chatID int64, user *telebot.User) error {
	if user.IsBot {
		return nil
	}

	// Присутствие сдвигает эпоху, даже если участник уже известен: его выход мог быть
	// пропущен, а начатая раньше проверка — застать его вне чата.
	h.markPresentLocked(chatID)

	author := message.SanitizeAuthor(user.Username, user.FirstName, user.ID, user.IsBot)
	err := h.users.Add(ctx, chat.TaggerUser{Author: author, ChatID: chatID, UserID: user.ID})
	if err != nil {
		return err
	}

	h.scheduleLocked(chatID)

	return nil
}

func (h *Handler) removeUser(ctx context.Context, chatID, userID int64) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.removeUserLocked(ctx, chatID, userID)
}

func (h *Handler) removeUserLocked(ctx context.Context, chatID, userID int64) error {
	// Выход сдвигает эпоху, даже если участник ещё не известен: начатая раньше
	// проверка могла застать его в чате.
	h.markRemovedLocked(chatID)

	if err := h.users.Remove(ctx, chatID, userID); err != nil {
		return err
	}

	if len(h.users.Users(chatID)) == 0 {
		h.unscheduleLocked(chatID)
	}

	return nil
}

func (h *Handler) markPresentLocked(chatID int64) {
	e := h.epochs[chatID]
	e.present++
	h.epochs[chatID] = e
}

func (h *Handler) markRemovedLocked(chatID int64) {
	e := h.epochs[chatID]
	e.removed++
	h.epochs[chatID] = e
}

// forgetChat удаляет все сведения о чате, из которого удалили бота.
func (h *Handler) forgetChat(ctx context.Context, chatID int64) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.markRemovedLocked(chatID)

	if err := h.users.RemoveChat(ctx, chatID); err != nil {
		return err
	}

	h.unscheduleLocked(chatID)

	return nil
}
