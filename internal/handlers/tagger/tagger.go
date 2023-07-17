package tagger

import (
	"container/heap"
	"context"
	"fmt"
	"sync"
	"time"

	"gopkg.in/telebot.v3"
)

type chat string

func (c chat) Recipient() string {
	return string(c)
}

type Handler struct {
	generator          messageGenerator
	log                logger
	random             randomizer
	nicknameRepository nicknameRepository
	bot                *telebot.Bot
	queue              *taggerQueue
	chatToUsers        map[string][]*telebot.User
	uniqueUsers        map[string]struct{}
	nextFromNano       int64
	nextInterval       int64
	mu                 sync.Mutex
}

func New(
	ctx context.Context,
	generator messageGenerator,
	nicknameRepository nicknameRepository,
	bot *telebot.Bot,
	log logger,
	random randomizer,
	nextFrom, nextTo time.Duration,
) *Handler {
	if nextFrom > nextTo {
		nextFrom, nextTo = nextTo, nextFrom
	}
	out := &Handler{
		generator:          generator,
		bot:                bot,
		nicknameRepository: nicknameRepository,
		log:                log,
		queue:              &taggerQueue{queue: make([]taggerJob, 0, 10)},
		chatToUsers:        make(map[string][]*telebot.User, 10),
		uniqueUsers:        make(map[string]struct{}, 2_000),
		nextFromNano:       nextFrom.Nanoseconds(),
		nextInterval:       nextTo.Nanoseconds() - nextFrom.Nanoseconds() + 1,
		random:             random,
	}

	go out.sender(ctx)

	return out
}

func (h *Handler) sender(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			h.queue.clean()
			return
		case <-time.After(time.Second):
		}

		if h.queue.Len() == 0 {
			continue
		}

		taskI := heap.Pop(h.queue)
		task := taskI.(taggerJob)
		if time.Now().Before(task.tagAt) {
			heap.Push(h.queue, task)
			continue
		}

		nicknames, err := h.nicknameRepository.GetEnabledNicknames()
		if err != nil {
			h.log.Warn(
				h.log.WithError(ctx, err),
				"can't get nicknames from repositody",
			)
			continue
		}

		nickname := nicknames[h.random.Intn(len(nicknames))]

		h.mu.Lock()
		users := h.chatToUsers[task.chatID]
		if len(users) == 0 {
			h.mu.Unlock()
			continue
		}

		index := h.random.Intn(len(users))
		user := users[index]

		text := fmt.Sprintf("[%s](tg://user?id=%d), %s", nickname, user.ID, h.generator.GetMessageText())
		h.mu.Unlock()

		if _, err := h.bot.Send(chat(task.chatID), text, telebot.ModeMarkdown); err != nil {
			h.log.Warn(
				h.log.WithFields(
					h.log.WithError(ctx, err),
					map[string]any{
						"chat_id":       task.chatID,
						"user_id":       user.ID,
						"user_username": user.Username,
					},
				),
				"can't send tagger message",
			)
		}

		heap.Push(
			h.queue,
			taggerJob{
				chatID: task.chatID,
				tagAt:  h.makeTagAt(),
			},
		)
	}
}

func (h *Handler) Slug() string {
	return "tagger"
}

func (h *Handler) Handle(ctx telebot.Context) error {
	chat := ctx.Chat()
	user := ctx.Sender()
	if chat == nil || user == nil {
		return nil
	}

	key := fmt.Sprintf("%d:%d", chat.ID, user.ID)

	h.mu.Lock()
	defer h.mu.Unlock()

	if _, notUnique := h.uniqueUsers[key]; notUnique {
		return nil
	}

	h.uniqueUsers[key] = struct{}{}

	h.chatToUsers[chat.Recipient()] = append(h.chatToUsers[chat.Recipient()], user)

	if len(h.chatToUsers[chat.Recipient()]) == 1 {
		heap.Push(
			h.queue,
			taggerJob{
				chatID: chat.Recipient(),
				tagAt:  h.makeTagAt(),
			},
		)
	}

	return nil
}

func (h *Handler) makeTagAt() time.Time {
	addNano := h.random.Int63n(h.nextInterval)
	addDuration := time.Duration(h.nextFromNano + addNano)
	return time.Now().Add(addDuration)
}
