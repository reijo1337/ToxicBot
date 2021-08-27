package greetings

import (
	"bufio"
	"fmt"
	"math/rand"
	"os"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

const filePathEnv = "GREETINGS_PATH"

type Greetings struct {
	messages []string
	r        *rand.Rand
}

func New() (*Greetings, error) {
	path := os.Getenv(filePathEnv)

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	out := Greetings{
		r: rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		out.messages = append(out.messages, scanner.Text())
	}

	if scanner.Err() != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	return &out, nil
}

func (g *Greetings) Handler(message *tgbotapi.Message) (tgbotapi.Chattable, error) {
	if message.NewChatMembers == nil {
		return nil, nil
	}
	randomIndex := g.r.Intn(len(g.messages))
	text := g.messages[randomIndex]

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ReplyToMessageID = message.MessageID

	return msg, nil
}
