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
	chatToUsers        map[string][]int64
	uniqueUsers        map[string]struct{}
	nextFromNano       int64
	nextInterval       int64
	mu                 sync.Mutex
	nicknames          []string
	nicknamesMu        sync.RWMutex
}

func New(
	ctx context.Context,
	generator messageGenerator,
	nicknameRepository nicknameRepository,
	bot *telebot.Bot,
	log logger,
	random randomizer,
	nextFrom, nextTo time.Duration,
	updateNicknames time.Duration,
) (*Handler, error) {
	if nextFrom > nextTo {
		nextFrom, nextTo = nextTo, nextFrom
	}
	out := &Handler{
		generator:          generator,
		bot:                bot,
		nicknameRepository: nicknameRepository,
		log:                log,
		queue:              &taggerQueue{queue: make([]taggerJob, 0, 10)},
		chatToUsers:        make(map[string][]int64, 10),
		uniqueUsers:        make(map[string]struct{}, 2_000),
		nextFromNano:       nextFrom.Nanoseconds(),
		nextInterval:       nextTo.Nanoseconds() - nextFrom.Nanoseconds() + 1,
		random:             random,
	}

	if err := out.updateNicknames(); err != nil {
		return nil, fmt.Errorf("can't init nicknames list: %w", err)
	}

	go func() {
		t := time.NewTimer(updateNicknames)
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := out.updateNicknames(); err != nil {
					log.Warn(
						log.WithFields(
							log.WithError(ctx, err),
							map[string]any{
								"handler": "tagger",
							},
						),
						"can't update nicknames",
					)
				}
			}
		}
	}()

	go out.sender(ctx)

	return out, nil
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

		text := fmt.Sprintf("[%s](tg://user?id=%d), %s", nickname, user, h.generator.GetMessageText())
		h.mu.Unlock()

		if _, err := h.bot.Send(chat(task.chatID), text, telebot.ModeMarkdown); err != nil {
			h.log.Warn(
				h.log.WithFields(
					h.log.WithError(ctx, err),
					map[string]any{
						"chat_id": task.chatID,
						"user_id": user,
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

func (h *Handler) updateNicknames() error {
	nicknames, err := h.nicknameRepository.GetEnabledNicknames()
	if err != nil {
		return fmt.Errorf("can't get nicknames from repository: %w", err)
	}

	h.nicknamesMu.Lock()
	defer h.nicknamesMu.Unlock()
	h.nicknames = make([]string, len(nicknames))
	copy(h.nicknames, nicknames)

	return nil
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

	h.addChatInfo(chat.Recipient(), user)

	return nil
}

func (h *Handler) addChatInfo(chat string, user *telebot.User) {
	key := fmt.Sprintf("%s:%d", chat, user.ID)
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, notUnique := h.uniqueUsers[key]; notUnique {
		return
	}

	h.uniqueUsers[key] = struct{}{}

	h.chatToUsers[chat] = append(h.chatToUsers[chat], user.ID)

	if len(h.chatToUsers[chat]) == 1 {
		heap.Push(
			h.queue,
			taggerJob{
				chatID: chat,
				tagAt:  h.makeTagAt(),
			},
		)
	}
}

func (h *Handler) makeTagAt() time.Time {
	addNano := h.random.Int63n(h.nextInterval)
	addDuration := time.Duration(h.nextFromNano + addNano)
	return time.Now().Add(addDuration)
}
