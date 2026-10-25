package tagger

import (
	"context"
	"fmt"
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

type Handler struct {
	ctx                context.Context
	generator          messageGenerator
	history            historyBuffer
	log                logger
	random             randomizer
	nicknameRepository nicknameRepository
	statIncer          statIncer
	settingsProvider   settingsProvider
	// users меняется только под mu: иначе список, эпохи и задача чата разойдутся.
	users userRegistry
	bot   *telebot.Bot
	// jobs хранит токен единственной задачи чата — в очереди или в работе у отправителя.
	jobs map[int64]uint64
	// epochs не очищаются, чтобы эпоха очищенного чата не начиналась заново.
	epochs       map[int64]chatEpoch
	queue        taggerQueue
	nicknames    []string
	nextFromNano int64
	nextInterval int64
	jobSeq       uint64
	nicknamesMu  sync.RWMutex
	mu           sync.Mutex
}

func New(
	ctx context.Context,
	generator messageGenerator,
	history historyBuffer,
	nicknameRepository nicknameRepository,
	users userRegistry,
	bot *telebot.Bot,
	log logger,
	random randomizer,
	statIncer statIncer,
	settingsProvider settingsProvider,
	nextFrom, nextTo time.Duration,
	updateNicknames time.Duration,
) (*Handler, error) {
	out := newHandler(
		ctx, generator, history, nicknameRepository, users, bot, log, random,
		statIncer, settingsProvider, nextFrom, nextTo,
	)

	if err := out.updateNicknames(); err != nil {
		return nil, fmt.Errorf("can't init nicknames list: %w", err)
	}

	out.scheduleKnownChats()

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

// newHandler собирает Handler без загрузки данных и фоновых горутин.
func newHandler(
	ctx context.Context,
	generator messageGenerator,
	history historyBuffer,
	nicknameRepository nicknameRepository,
	users userRegistry,
	bot *telebot.Bot,
	log logger,
	random randomizer,
	statIncer statIncer,
	settingsProvider settingsProvider,
	nextFrom, nextTo time.Duration,
) *Handler {
	if nextFrom > nextTo {
		nextFrom, nextTo = nextTo, nextFrom
	}
	return &Handler{
		ctx:                ctx,
		generator:          generator,
		history:            history,
		bot:                bot,
		nicknameRepository: nicknameRepository,
		users:              users,
		log:                log,
		statIncer:          statIncer,
		settingsProvider:   settingsProvider,
		jobs:               make(map[int64]uint64, 10),
		epochs:             make(map[int64]chatEpoch, 10),
		nextFromNano:       nextFrom.Nanoseconds(),
		nextInterval:       nextTo.Nanoseconds() - nextFrom.Nanoseconds() + 1,
		random:             random,
	}
}

func (h *Handler) sender(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			h.tick(ctx, now)
		}
	}
}

// tick делает одну попытку тега, если срок ближайшей задачи наступил. После попытки
// у непустого чата всегда остаётся ровно одна следующая задача.
func (h *Handler) tick(ctx context.Context, now time.Time) {
	attempt, ok := h.takeDue(now)
	if !ok {
		return
	}
	defer h.finishJob(attempt.job)

	h.tag(ctx, attempt)
}

func (h *Handler) tag(ctx context.Context, attempt tagAttempt) {
	chatID, userID := attempt.job.chatID, attempt.userID
	logCtx := h.log.WithFields(ctx, map[string]any{
		"chat_id": chatID,
		"user_id": userID,
	})
	// Ошибка чтения настроек не выключает теггер: пинг идёт с нулевой вероятностью LLM.
	var aiChance float32
	settings, err := h.settingsProvider.GetForChat(ctx, chatID)
	switch {
	case err != nil:
		h.log.Warn(h.log.WithError(logCtx, err), "can't get chat settings for tagger")
	case !settings.TaggerEnabled:
		return
	default:
		aiChance = settings.AIChance
	}

	tgChat := &telebot.Chat{ID: chatID}

	member, err := h.bot.ChatMemberOf(tgChat, &telebot.User{ID: userID})
	if err != nil {
		h.log.Warn(h.log.WithError(logCtx, err), "can't check tagger candidate membership")
		return
	}

	switch membershipOf(member) {
	case membershipGone:
		if err := h.dropGoneCandidate(ctx, attempt); err != nil {
			h.log.Warn(h.log.WithError(logCtx, err), "can't forget left tagger candidate")
		}
		return
	case membershipUnknown:
		h.log.Warn(logCtx, "tagger candidate membership is unknown")
		return
	case membershipPresent:
	}

	nicknames, err := h.nicknameRepository.GetEnabledNicknames()
	if err != nil {
		h.log.Warn(h.log.WithError(logCtx, err), "can't get nicknames from repositody")
		return
	}
	if len(nicknames) == 0 {
		h.log.Warn(logCtx, "no enabled nicknames for tagger")
		return
	}

	h.mu.Lock()
	nickname := nicknames[h.random.Intn(len(nicknames))]
	h.mu.Unlock()

	text := h.buildTag(chatID, userID, nickname, aiChance)

	if !h.canSend(attempt) {
		return
	}

	go h.statIncer.Inc(h.ctx, chatID, userID, stats.PersonalOperationType)

	if _, err := h.bot.Send(tgChat, text, telebot.ModeMarkdown); err != nil {
		h.log.Warn(h.log.WithError(logCtx, err), "can't send tagger message")
	}
}

// buildTag generates the insult text for a scheduled tag and owns the
// timer-driven root span (there is no incoming update / telebot.Context here).
func (h *Handler) buildTag(chatID, user int64, nickname string, aiChance float32) string {
	ctx, span := tracing.Tracer().Start(context.Background(), "tagger")
	defer span.End()
	span.SetAttributes(
		attribute.String("trigger", "timer"),
		attribute.Int64("chat.id", chatID),
		attribute.Int64("user.id", user),
		attribute.String("nickname", nickname),
	)

	author, _ := h.users.Author(chatID, user)
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
	if chat == nil || user == nil {
		return nil
	}

	return h.observe(h.ctx, chat, user)
}

func (h *Handler) makeTagAt() time.Time {
	addNano := h.random.Int63n(h.nextInterval)
	addDuration := time.Duration(h.nextFromNano + addNano)
	return time.Now().Add(addDuration)
}
