package max

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/reijo1337/ToxicBot/internal/handlers/on_text"
	"github.com/reijo1337/ToxicBot/internal/storage"

	"gopkg.in/telebot.v3"
)

const (
	idEnv = "MAX_ID"
)

type max struct {
	s  storage.Manager
	r  *rand.Rand
	id int64
}

func New(storage storage.Manager) (on_text.SubHandler, error) {
	id, err := strconv.ParseInt(os.Getenv(idEnv), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse igor id from env: %w", err)
	}

	return &max{
		r:  rand.New(rand.NewSource(time.Now().UnixNano())),
		id: id,
		s:  storage,
	}, nil
}

func (i *max) Slug() string {
	return "max"
}

func (i *max) Handle(ctx telebot.Context) error {
	user := ctx.Sender()
	if user == nil || user.ID != i.id {
		return nil
	}

	if i.r.Intn(750) != 0 {
		return nil
	}

	maxPhrases, err := i.s.GetMaxs()
	if err != nil {
		return err
	}
	maxPhrases = maxPhrases.GetEnabled()

	if idx := i.r.Intn(len(maxPhrases)); idx == 0 {
		return ctx.Reply(maxPhrases[idx].Text)
	}

	return nil
}
