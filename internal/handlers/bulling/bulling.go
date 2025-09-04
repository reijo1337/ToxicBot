package bulling

import (
	"container/list"
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/reijo1337/ToxicBot/internal/features/stats"
	"github.com/reijo1337/ToxicBot/pkg/pointer"
	"gopkg.in/telebot.v3"
)

type Handler struct {
	ctx        context.Context
	generator  messageGenerator
	statIncer  statIncer
	r          *rand.Rand
	msgCount   map[string]*list.List
	cooldown   map[string]time.Time
	muCount    sync.Mutex
	muCooldown sync.Mutex

	thresholdCount int
	thresholdTime  time.Duration
	cooldownTime   time.Duration
}

func New(
	ctx context.Context,
	generator messageGenerator,
	statIncer statIncer,
	thresholdCount int,
	thresholdTime time.Duration,
	cooldownTime time.Duration,
) (*Handler, error) {
	return &Handler{
		ctx:       ctx,
		generator: generator,
		statIncer: statIncer,
		msgCount:  make(map[string]*list.List),
		cooldown:  make(map[string]time.Time),
		r:         rand.New(rand.NewSource(time.Now().UnixNano())),

		thresholdCount: thresholdCount,
		thresholdTime:  thresholdTime,
		cooldownTime:   cooldownTime,
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

	key := fmt.Sprintf("%d:%d", chat.ID, user.ID)

	isCooldown := b.isCooldown(key)
	isMsgThreshold := b.isMsgThreshold(key, ctx.Message().Time())
	isReplyOrMention := isReplyOrMention(ctx)

	if !isReplyOrMention {
		if isCooldown || !isMsgThreshold {
			return nil
		}
	}

	// КД на булинг
	b.setCooldown(key)

	text := b.generator.GetMessageText(ctx.Message().Text)

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

func (b *Handler) setCooldown(key string) {
	b.muCooldown.Lock()
	b.cooldown[key] = time.Now().Add(b.cooldownTime)
	b.muCooldown.Unlock()
}

func (b *Handler) isMsgThreshold(key string, msgTime time.Time) bool {
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

		if time.Since(t) > b.thresholdTime {
			b.msgCount[key].Remove(e)
		}
	}

	return b.msgCount[key].Len() >= b.thresholdCount
}

func isReplyOrMention(ctx telebot.Context) bool {
	me := ctx.Bot().Me
	isMention := strings.Contains(ctx.Message().Text, "@"+me.Username)
	isReply := pointer.From(pointer.From(ctx.Message().ReplyTo).Sender).ID == me.ID
	return isMention || isReply
}
