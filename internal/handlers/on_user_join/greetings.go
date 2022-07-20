package on_user_join

import (
	"math/rand"
	"os"
	"time"

	"github.com/reijo1337/ToxicBot/internal/utils"
	"gopkg.in/telebot.v3"
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

func (g *Greetings) Handle(ctx telebot.Context) error {
	randomIndex := g.r.Intn(len(g.messages))
	text := g.messages[randomIndex]
	ctx.Reply(text)

	return nil
}
