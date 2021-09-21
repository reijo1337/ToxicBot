package bulling

import (
	"bufio"
	"container/list"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/sirupsen/logrus"
)

type Bulling struct {
	cfg      config
	log      *logrus.Logger
	messages []string
	r        *rand.Rand

	msgCount map[string]*list.List
	muCount  sync.Mutex

	cooldown   map[string]time.Time
	muCooldown sync.Mutex
}

func New(log *logrus.Logger) (*Bulling, error) {
	out := Bulling{
		r:        rand.New(rand.NewSource(time.Now().UnixNano())),
		msgCount: make(map[string]*list.List),
		cooldown: make(map[string]time.Time),
		log:      log,
	}

	if err := out.parseConfig(); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	file, err := os.Open(out.cfg.FilePath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		out.messages = append(out.messages, scanner.Text())
	}

	if scanner.Err() != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	return &out, nil
}

func (b *Bulling) Handler(message *tgbotapi.Message) (tgbotapi.Chattable, error) {
	if message.Chat == nil || !message.Chat.IsGroup() && !message.Chat.IsSuperGroup() {
		return nil, nil
	}

	now := time.Now()
	key := fmt.Sprintf("%d:%d", message.Chat.ID, message.From.ID)

	b.log.Infof("bulling; message from %+v; key %s", *message.From, key)

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

	b.log.Infof("bulling %s; check time list; start len %d", key, b.msgCount[key].Len())

	// Удаляем инфу, старше порога времени из конфига
	for e := b.msgCount[key].Front(); e != nil; e = e.Next() {
		t := e.Value.(time.Time)

		if now.Sub(t) > b.cfg.ThresholdTime {
			b.msgCount[key].Remove(e)
			b.log.Infof("bulling %s; check time list; remove time %s; now len %d", key, t, b.msgCount[key].Len())
		}
	}

	b.log.Infof("bulling %s; check time list; final len %d", key, b.msgCount[key].Len())

	// Булим
	if b.msgCount[key].Len() >= b.cfg.ThresholdCount {
		// КД на булинг
		b.setCooldown(key)

		randomIndex := b.r.Intn(len(b.messages))
		text := b.messages[randomIndex]

		msg := tgbotapi.NewMessage(message.Chat.ID, text)
		msg.ReplyToMessageID = message.MessageID

		return msg, nil
	}

	return nil, nil
}

func (b *Bulling) isCooldown(key string) bool {
	b.muCooldown.Lock()
	defer b.muCooldown.Unlock()

	b.log.Infof("is cooldown %s", key)

	t, ok := b.cooldown[key]
	if !ok {
		b.log.Infof("is cooldown %s; has no key", key)
		return false
	}

	if time.Now().After(t) {
		b.log.Infof("is cooldown %s; has expired key", key)
		delete(b.cooldown, key)
		return false
	}

	b.log.Infof("is cooldown %s; has actual key", key)
	return true
}

func (b *Bulling) setCooldown(key string) {
	b.muCooldown.Lock()
	b.cooldown[key] = time.Now().Add(b.cfg.Cooldown)
	b.muCooldown.Unlock()
}
