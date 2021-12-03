package bulling

import (
	"container/list"
	"fmt"
	"github.com/reijo1337/ToxicBot/internal/utils"
	"math/rand"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/mb-14/gomarkov"
)

type Bulling struct {
	cfg      config
	messages []string
	r        *rand.Rand

	msgCount map[string]*list.List
	muCount  sync.Mutex

	cooldown   map[string]time.Time
	muCooldown sync.Mutex

	chain *gomarkov.Chain
}

func New() (*Bulling, error) {
	out := Bulling{
		r:        rand.New(rand.NewSource(time.Now().UnixNano())),
		msgCount: make(map[string]*list.List),
		cooldown: make(map[string]time.Time),
		chain:    gomarkov.NewChain(1),
	}

	if err := out.parseConfig(); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	err := utils.ReadFile(out.cfg.FilePath, &out.messages)
	if err != nil {
		return nil, err
	}

	for _, message := range out.messages {
		out.chain.Add(strings.Split(strings.Trim(message, " "), " "))
	}


	return &out, nil
}

func (b *Bulling) Handler(message *tgbotapi.Message) (tgbotapi.Chattable, error) {
	if message.Chat == nil || !message.Chat.IsGroup() && !message.Chat.IsSuperGroup() {
		return nil, nil
	}

	now := time.Now()
	key := fmt.Sprintf("%d:%d", message.Chat.ID, message.From.ID)

	// Уже булили, надо подождать
	if b.isCooldown(key) {
		return nil, nil
	}

	b.muCount.Lock()
	defer b.muCount.Unlock()

	// Накапливаем инфу о сообщениях
	if _, ok := b.msgCount[key]; !ok {
		b.msgCount[key] = list.New()
	}
	b.msgCount[key].PushBack(message.Time())

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

		msg := tgbotapi.NewMessage(message.Chat.ID, text)
		msg.ReplyToMessageID = message.MessageID

		return msg, nil
	}

	return nil, nil
}

func (b *Bulling) isCooldown(key string) bool {
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

func (b *Bulling) setCooldown(key string) {
	b.muCooldown.Lock()
	b.cooldown[key] = time.Now().Add(b.cfg.Cooldown)
	b.muCooldown.Unlock()
}

func (b *Bulling) getMessageText() string {
	if b.r.Float32() <= b.cfg.MarkovChance {
		text, err := b.generateDegenerate()
		if err == nil {
			return text
		}
	}

	randomIndex := b.r.Intn(len(b.messages))
	return b.messages[randomIndex]
}

func (b *Bulling) generateDegenerate() (string, error) {
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
