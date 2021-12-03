package greetings

import (
	"github.com/reijo1337/ToxicBot/internal/utils"
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
	out := Greetings{
		r: rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	path := os.Getenv(filePathEnv)

	messages, err := utils.ReadFile(path)
	if err != nil {
		return nil, err
	}

	out.messages = messages
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
