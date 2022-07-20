package igor

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/reijo1337/ToxicBot/internal/handlers/on_text"
	"gopkg.in/telebot.v3"
)

const (
	idEnv   = "IGOR_ID"
	fileEnv = "IGOR_FILE_PATH"
)

type igor struct {
	r    *rand.Rand
	id   int64
	text string
}

func New() (on_text.SubHandler, error) {
	id, err := strconv.ParseInt(os.Getenv(idEnv), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse igor id from env: %w", err)
	}

	data, err := ioutil.ReadFile(os.Getenv(fileEnv))
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	return &igor{
		r:    rand.New(rand.NewSource(time.Now().UnixNano())),
		id:   id,
		text: string(data),
	}, nil
}

func (i *igor) Slug() string {
	return "igor"
}

func (i *igor) Handle(ctx telebot.Context) error {
	user := ctx.Sender()
	if user == nil || user.ID != i.id {
		return nil
	}

	if i.r.Intn(750) != 0 {
		return nil
	}

	return ctx.Reply(i.text)
}
