package igor

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"strconv"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

const (
	idEnv   = "IGOR_ID"
	fileEnv = "IGOR_FILE_PATH"
)

type Igor struct {
	r    *rand.Rand
	id   int
	text string
}

func New() (*Igor, error) {
	id, err := strconv.Atoi(os.Getenv(idEnv))
	if err != nil {
		return nil, fmt.Errorf("parse igor id from env: %w", err)
	}

	data, err := ioutil.ReadFile(os.Getenv(fileEnv))
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	return &Igor{
		r:    rand.New(rand.NewSource(time.Now().UnixNano())),
		id:   id,
		text: string(data),
	}, nil
}

func (i *Igor) Handler(message *tgbotapi.Message) (tgbotapi.Chattable, error) {
	if message.From.ID != i.id {
		return nil, nil
	}

	if i.r.Intn(2) != 0 {
		return nil, nil
	}

	msg := tgbotapi.NewMessage(message.Chat.ID, i.text)
	msg.ReplyToMessageID = message.MessageID

	return msg, nil
}
