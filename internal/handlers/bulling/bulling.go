package bulling

import (
	"container/list"
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/reijo1337/ToxicBot/internal/chatsettings"
	"github.com/reijo1337/ToxicBot/internal/features/stats"
	"github.com/reijo1337/ToxicBot/internal/message"
	"github.com/reijo1337/ToxicBot/pkg/pointer"
	"gopkg.in/telebot.v3"
)

const historyBufferSize = 50

type chatMessage struct {
	Author string
	Text   string
}

type settingsProvider interface {
	GetForChat(ctx context.Context, chatID int64) (*chatsettings.Settings, error)
}

type Handler struct {
	ctx              context.Context
	generator        messageGenerator
	statIncer        statIncer
	settingsProvider settingsProvider
	r                *rand.Rand
	msgCount         map[string]*list.List
	cooldown         map[string]time.Time
	muCount          sync.Mutex
	muCooldown       sync.Mutex
	history          map[int64][]chatMessage
	muHistory        sync.Mutex
}

func New(
	ctx context.Context,
	generator messageGenerator,
	statIncer statIncer,
	settingsProvider settingsProvider,
) (*Handler, error) {
	return &Handler{
		ctx:              ctx,
		generator:        generator,
		statIncer:        statIncer,
		settingsProvider: settingsProvider,
		msgCount:         make(map[string]*list.List),
		cooldown:         make(map[string]time.Time),
		history:          make(map[int64][]chatMessage),
		r:                rand.New(rand.NewSource(time.Now().UnixNano())),
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

	author := user.FirstName
	if user.Username != "" {
		author = "@" + user.Username
	}
	if user.IsBot {
		author = "Админ какого-то канала"
	}

	b.addToHistory(chat.ID, author, ctx.Message().Text)

	settings, err := b.settingsProvider.GetForChat(b.ctx, chat.ID)
	if err != nil {
		return fmt.Errorf("can't get chat settings: %w", err)
	}

	key := fmt.Sprintf("%d:%d", chat.ID, user.ID)

	isCooldown := b.isCooldown(key)
	isMsgThreshold := b.isMsgThreshold(
		key,
		ctx.Message().Time(),
		settings.ThresholdCount,
		settings.ThresholdTime,
	)
	isReplyOrMention := isReplyOrMention(ctx)

	if !isReplyOrMention {
		if isCooldown || !isMsgThreshold {
			return nil
		}
	}

	// КД на булинг
	b.setCooldown(key, settings.Cooldown)

	history := b.getHistory(chat.ID)
	text := b.generator.GetMessageTextWithHistory(
		history,
		message.HistoryMessage{Author: author, Text: ctx.Message().Text},
		settings.AIChance,
	)

	go b.statIncer.Inc(
		b.ctx,
		chat.ID,
		user.ID,
		stats.OnTextOperationType,
		stats.WithGenStrategy(text.Strategy),
	)

	if err := ctx.Notify(telebot.Typing); err != nil {
		return err
	}
	time.Sleep(time.Duration((float64(b.r.Intn(3)) + b.r.Float64()) * 1_000_000_000))

	return ctx.Reply(text.Message)
}

func (b *Handler) addToHistory(chatID int64, author, text string) {
	b.muHistory.Lock()
	defer b.muHistory.Unlock()

	buf := b.history[chatID]
	buf = append(buf, chatMessage{Author: author, Text: text})
	if len(buf) > historyBufferSize {
		buf = buf[len(buf)-historyBufferSize:]
	}
	b.history[chatID] = buf
}

func (b *Handler) getHistory(chatID int64) []message.HistoryMessage {
	b.muHistory.Lock()
	defer b.muHistory.Unlock()

	buf := b.history[chatID]
	out := make([]message.HistoryMessage, len(buf))
	for i, m := range buf {
		out[i] = message.HistoryMessage{Author: m.Author, Text: m.Text}
	}
	return out
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
