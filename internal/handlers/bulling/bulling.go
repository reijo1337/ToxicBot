package bulling

import (
	"container/list"
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/reijo1337/ToxicBot/internal/features/chatsettings"
	"github.com/reijo1337/ToxicBot/internal/features/message"
	"github.com/reijo1337/ToxicBot/internal/features/stats"
	"github.com/reijo1337/ToxicBot/pkg/pointer"
	"github.com/reijo1337/ToxicBot/pkg/tracing"
	"go.opentelemetry.io/otel/attribute"
	"gopkg.in/telebot.v3"
)

type settingsProvider interface {
	GetForChat(ctx context.Context, chatID int64) (*chatsettings.Settings, error)
}

type Handler struct {
	ctx              context.Context
	generator        messageGenerator
	statIncer        statIncer
	settingsProvider settingsProvider
	history          historyBuffer
	replier          botReplier
	r                *rand.Rand
	msgCount         map[string]*list.List
	cooldown         map[string]time.Time
	botAuthor        string
	muCount          sync.Mutex
	muCooldown       sync.Mutex
}

func New(
	ctx context.Context,
	generator messageGenerator,
	statIncer statIncer,
	settingsProvider settingsProvider,
	historyBuffer historyBuffer,
	replier botReplier,
	botAuthor string,
) (*Handler, error) {
	return &Handler{
		ctx:              ctx,
		generator:        generator,
		statIncer:        statIncer,
		settingsProvider: settingsProvider,
		history:          historyBuffer,
		replier:          replier,
		msgCount:         make(map[string]*list.List),
		cooldown:         make(map[string]time.Time),
		r:                rand.New(rand.NewSource(time.Now().UnixNano())),
		botAuthor:        botAuthor,
	}, nil
}

func (b *Handler) Slug() string {
	return "bulling"
}

func (b *Handler) Handle(ctx telebot.Context) error {
	chat := ctx.Chat()
	user := ctx.Sender()

	if chat == nil || user == nil {
		return nil
	}

	spanCtx, span := tracing.StartHandlerSpan(ctx, b.Slug())
	defer span.End()

	author := message.SanitizeAuthor(user.Username, user.FirstName, user.ID, user.IsBot)

	msg := ctx.Message()
	replyToID := 0
	if msg.ReplyTo != nil {
		replyToID = msg.ReplyTo.ID
	}

	userEntry := chathistory.Entry{
		ID:        msg.ID,
		Time:      msg.Time(),
		Author:    author,
		Text:      msg.Text,
		ReplyToID: replyToID,
		FromBot:   false,
	}

	settings, err := b.settingsProvider.GetForChat(b.ctx, chat.ID)
	if err != nil {
		return fmt.Errorf("can't get chat settings: %w", err)
	}

	key := fmt.Sprintf("%d:%d", chat.ID, user.ID)

	isCooldown := b.isCooldown(key)
	isMsgThreshold := b.isMsgThreshold(
		key,
		msg.Time(),
		settings.ThresholdCount,
		settings.ThresholdTime,
	)
	isReplyOrMention := isReplyOrMention(ctx)

	if !isReplyOrMention {
		if isCooldown || !isMsgThreshold {
			span.SetAttributes(
				attribute.String("outcome", "skip"),
				attribute.Bool("cooldown", isCooldown),
				attribute.Bool("threshold_met", isMsgThreshold),
			)
			// Non-triggering path: record the message for future context.
			b.history.Add(chat.ID, userEntry)
			return nil
		}
	}

	b.setCooldown(key, settings.Cooldown)

	trigger := "threshold"
	if isReplyOrMention {
		trigger = "reply_or_mention"
	}
	span.SetAttributes(attribute.String("outcome", "react"), attribute.String("trigger", trigger))

	history := b.history.Get(chat.ID)
	history = append(history, userEntry)
	result := b.generator.GetMessageTextWithHistory(
		spanCtx,
		history,
		settings.AIChance,
		false,
	)
	span.SetAttributes(attribute.String("strategy", message.StrategyName(result.Strategy)))

	go b.statIncer.Inc(
		b.ctx,
		chat.ID,
		user.ID,
		stats.OnTextOperationType,
		stats.WithGenStrategy(result.Strategy),
	)

	if err := ctx.Notify(telebot.Typing); err != nil {
		return err
	}
	time.Sleep(time.Duration((float64(b.r.Intn(3)) + b.r.Float64()) * 1_000_000_000))

	sent, err := b.replier.Reply(msg, result.Message)
	if err != nil {
		// Reply failed — still record the user's turn so future context is not
		// missing this message.
		b.history.Add(chat.ID, userEntry)
		return err
	}
	span.SetAttributes(attribute.Int("telegram.sent_message_id", sent.ID))

	botEntry := chathistory.Entry{
		ID:        sent.ID,
		Time:      time.Now(),
		Author:    b.botAuthor,
		Text:      result.Message,
		ReplyToID: msg.ID,
		FromBot:   true,
	}

	b.history.AddAll(chat.ID, userEntry, botEntry)

	return nil
}

func (b *Handler) isCooldown(key string) bool {
	b.muCooldown.Lock()
	defer b.muCooldown.Unlock()

	t, ok := b.cooldown[key]
	if !ok {
		return false
	}

	if time.Now().After(t) {
		delete(b.cooldown, key)
		return false
	}

	return true
}

func (b *Handler) setCooldown(key string, cooldownTime time.Duration) {
	b.muCooldown.Lock()
	b.cooldown[key] = time.Now().Add(cooldownTime)
	b.muCooldown.Unlock()
}

func (b *Handler) isMsgThreshold(
	key string,
	msgTime time.Time,
	thresholdCount int,
	thresholdTime time.Duration,
) bool {
	b.muCount.Lock()
	defer b.muCount.Unlock()

	// Накапливаем инфу о сообщениях
	if _, ok := b.msgCount[key]; !ok {
		b.msgCount[key] = list.New()
	}

	b.msgCount[key].PushBack(msgTime)

	// Удаляем инфу, старше порога времени из конфига
	var next *list.Element
	for e := b.msgCount[key].Front(); e != nil; e = next {
		next = e.Next()
		t := e.Value.(time.Time)

		if time.Since(t) > thresholdTime {
			b.msgCount[key].Remove(e)
		}
	}

	return b.msgCount[key].Len() >= thresholdCount
}

func isReplyOrMention(ctx telebot.Context) bool {
	me := ctx.Bot().Me
	isMention := strings.Contains(ctx.Message().Text, "@"+me.Username)
	isReply := pointer.From(pointer.From(ctx.Message().ReplyTo).Sender).ID == me.ID
	return isMention || isReply
}
