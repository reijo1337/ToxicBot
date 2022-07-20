package bulling

import (
	"container/list"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/reijo1337/ToxicBot/internal/handlers/on_text"
	"github.com/reijo1337/ToxicBot/internal/utils"
	"gopkg.in/telebot.v3"

	"github.com/mb-14/gomarkov"
)

type bulling struct {
	cfg      config
	messages []string
	r        *rand.Rand

	msgCount map[string]*list.List
	muCount  sync.Mutex

	cooldown   map[string]time.Time
	muCooldown sync.Mutex

	chain *gomarkov.Chain
}

func New() (on_text.SubHandler, error) {
	out := bulling{
		r:        rand.New(rand.NewSource(time.Now().UnixNano())),
		msgCount: make(map[string]*list.List),
		cooldown: make(map[string]time.Time),
		chain:    gomarkov.NewChain(1),
	}

	if err := out.parseConfig(); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	messages, err := utils.ReadFile(out.cfg.FilePath)
	if err != nil {
		return nil, err
	}

	out.messages = messages

	for _, message := range out.messages {
		out.chain.Add(strings.Split(strings.Trim(message, " "), " "))
	}

	return &out, nil
}

func (b *bulling) Slug() string {
	return "bulling"
}

func (b *bulling) Handle(ctx telebot.Context) error {
	chat := ctx.Chat()
	user := ctx.Sender()
	if chat == nil || user == nil {
		return nil
	}

	now := time.Now()
	key := fmt.Sprintf("%d:%d", chat.ID, user.ID)

	// Уже булили, надо подождать
	if b.isCooldown(key) {
		return nil
	}

	b.muCount.Lock()
	defer b.muCount.Unlock()

	// Накапливаем инфу о сообщениях
	if _, ok := b.msgCount[key]; !ok {
		b.msgCount[key] = list.New()
	}
	b.msgCount[key].PushBack(ctx.Message().Time())

	// Удаляем инфу, старше порога времени из конфига
	var next *list.Element
	for e := b.msgCount[key].Front(); e != nil; e = next {
		next = e.Next()
		t := e.Value.(time.Time)

		if now.Sub(t) > b.cfg.ThresholdTime {
			b.msgCount[key].Remove(e)
		}
	}

	// Булим
	if b.msgCount[key].Len() >= b.cfg.ThresholdCount {
		// КД на булинг
		b.setCooldown(key)

		text := b.getMessageText()

		ctx.Reply(text)

		return nil
	}

	return nil
}

func (b *bulling) isCooldown(key string) bool {
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

func (b *bulling) setCooldown(key string) {
	b.muCooldown.Lock()
	b.cooldown[key] = time.Now().Add(b.cfg.Cooldown)
	b.muCooldown.Unlock()
}

func (b *bulling) getMessageText() string {
	if b.r.Float32() <= b.cfg.MarkovChance {
		text, err := b.generateDegenerate()
		if err == nil {
			return text
		}
	}

	randomIndex := b.r.Intn(len(b.messages))
	return b.messages[randomIndex]
}

func (b *bulling) generateDegenerate() (string, error) {
	tokens := []string{gomarkov.StartToken}
	for tokens[len(tokens)-1] != gomarkov.EndToken {
		next, err := b.chain.Generate(tokens[(len(tokens) - 1):])
		if err != nil {
			return "", fmt.Errorf("can't generate next token: %w", err)
		}
		tokens = append(tokens, next)
	}
	return strings.Join(tokens[1:len(tokens)-1], " "), nil
}
