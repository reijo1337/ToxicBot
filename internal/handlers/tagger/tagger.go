package tagger

import (
	"container/heap"
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/reijo1337/ToxicBot/internal/features/message"
	"github.com/reijo1337/ToxicBot/internal/features/stats"
	"github.com/reijo1337/ToxicBot/pkg/tracing"
	"go.opentelemetry.io/otel/attribute"
	"gopkg.in/telebot.v3"
)

// Кличка выбирается случайно и не идентифицирует автора в истории.
const tagSteering = `Дополнительные правила именно для этой реплики:
- Это пинг по таймеру. Сейчас никто ничего не писал; вся история — только контекст, на неё напрямую не отвечай. Правила ответа на последнюю реплику здесь не применяются.
- Ты сам обращаешься к участнику с подписью «%s» в атрибуте from истории. Именно он — адресат этой реплики.
- «%s» — случайное обращение перед твоей репликой, а не настоящее имя участника и не подпись автора в истории. Не ищи адресата по этой кличке.
- Не приписывай адресату слова и поступки других участников. Его собственные реплики определяй только по его подписи from. Если его реплик в истории нет или авторство неоднозначно, используй общую тему, не утверждая, что он что-то сказал или сделал.
- Отталкивайся от того, что недавно обсуждали в чате, или от того, что адресат отмалчивается.
- Не начинай реплику с обращения по кличке или имени — обращение уже стоит перед твоей репликой.`

func buildTagSteering(author, nickname string) string {
	return fmt.Sprintf(
		tagSteering,
		message.SanitizeText(author, 64),
		message.SanitizeText(nickname, 64),
	)
}

type chat string

func (c chat) Recipient() string {
	return string(c)
}

type Handler struct {
	ctx                context.Context
	generator          messageGenerator
	history            historyBuffer
	log                logger
	random             randomizer
	nicknameRepository nicknameRepository
	statIncer          statIncer
	settingsProvider   settingsProvider
	chatToUsers        map[string][]int64
	queue              *taggerQueue
	bot                *telebot.Bot
	uniqueUsers        map[string]string
	nicknames          []string
	nextFromNano       int64
	nextInterval       int64
	nicknamesMu        sync.RWMutex
	mu                 sync.Mutex
}

func New(
	ctx context.Context,
	generator messageGenerator,
	history historyBuffer,
	nicknameRepository nicknameRepository,
	bot *telebot.Bot,
	log logger,
	random randomizer,
	statIncer statIncer,
	settingsProvider settingsProvider,
	nextFrom, nextTo time.Duration,
	updateNicknames time.Duration,
) (*Handler, error) {
	if nextFrom > nextTo {
		nextFrom, nextTo = nextTo, nextFrom
	}
	out := &Handler{
		ctx:                ctx,
		generator:          generator,
		history:            history,
		bot:                bot,
		nicknameRepository: nicknameRepository,
		log:                log,
		statIncer:          statIncer,
		settingsProvider:   settingsProvider,
		queue:              &taggerQueue{queue: make([]taggerJob, 0, 10)},
		chatToUsers:        make(map[string][]int64, 10),
		uniqueUsers:        make(map[string]string, 2_000),
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

		h.mu.Unlock()
		chatIDint, _ := strconv.ParseInt(task.chatID, 10, 64)
		text := h.buildTag(chatIDint, user, nickname)

		go h.statIncer.Inc(h.ctx, chatIDint, user, stats.PersonalOperationType)

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

// buildTag generates the insult text for a scheduled tag and owns the
// timer-driven root span (there is no incoming update / telebot.Context here).
func (h *Handler) buildTag(chatID, user int64, nickname string) string {
	ctx, span := tracing.Tracer().Start(context.Background(), "tagger")
	defer span.End()
	span.SetAttributes(
		attribute.String("trigger", "timer"),
		attribute.Int64("chat.id", chatID),
		attribute.Int64("user.id", user),
		attribute.String("nickname", nickname),
	)

	var aiChance float32
	if s, err := h.settingsProvider.GetForChat(ctx, chatID); err == nil {
		aiChance = s.AIChance
	}

	h.mu.Lock()
	author := h.uniqueUsers[fmt.Sprintf("%d:%d", chatID, user)]
	h.mu.Unlock()
	if author == "" {
		author = message.SanitizeAuthor("", "", user, false)
	}

	genResult := h.generator.GetMessageTextForTag(
		ctx,
		h.history.Get(chatID),
		aiChance,
		buildTagSteering(author, nickname),
	)
	span.SetAttributes(tracing.ContentAttr("output", genResult.Message))

	text := stripLeadingNickname(genResult.Message, nickname)
	return fmt.Sprintf("[%s](tg://user?id=%d), %s", nickname, user, text)
}

// stripLeadingNickname: модель начинает реплику с клички вопреки указанию, а
// упоминание ставит тегер — без зачистки выходит «[Валера](…), Валера, …».
func stripLeadingNickname(text, nickname string) string {
	trimmed := strings.TrimSpace(text)
	if nickname == "" || len(trimmed) < len(nickname) ||
		!strings.EqualFold(trimmed[:len(nickname)], nickname) {
		return text
	}
	after := trimmed[len(nickname):]
	if next, _ := utf8.DecodeRuneInString(after); unicode.IsLetter(next) || unicode.IsDigit(next) {
		return text // кличка — лишь начало другого слова
	}
	rest := strings.TrimLeftFunc(after, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r) || r == '—'
	})
	if rest == "" {
		return text
	}
	first, size := utf8.DecodeRuneInString(rest)
	return string(unicode.ToUpper(first)) + rest[size:]
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
	member, err := h.bot.ChatMemberOf(chat, user)
	if err != nil {
		return fmt.Errorf("can't check if user is member of chat: %w", err)
	}

	if chat == nil ||
		user == nil ||
		member == nil ||
		member.Role == telebot.Left ||
		member.Role == telebot.Kicked {
		return nil
	}

	h.addChatInfo(chat.Recipient(), user)

	return nil
}

func (h *Handler) addChatInfo(chat string, user *telebot.User) {
	if user.IsBot {
		return
	}

	key := fmt.Sprintf("%s:%d", chat, user.ID)

	h.mu.Lock()
	defer h.mu.Unlock()

	_, notUnique := h.uniqueUsers[key]
	h.uniqueUsers[key] = message.SanitizeAuthor(user.Username, user.FirstName, user.ID, user.IsBot)
	if notUnique {
		return
	}

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
