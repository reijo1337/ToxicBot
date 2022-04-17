package sticker_reactions

import (
	"fmt"
	"math/rand"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/reijo1337/ToxicBot/internal/utils"
)

type StickerReactions struct {
	cfg      config
	stickers []string
	r        *rand.Rand
}

func New() (*StickerReactions, error) {
	out := StickerReactions{
		r: rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	if err := out.parseConfig(); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	stickers, err := utils.ReadFile(out.cfg.FilePath)
	if err != nil {
		return nil, err
	}

	out.stickers = stickers

	return &out, nil
}

func (sr *StickerReactions) Handler(message *tgbotapi.Message) (tgbotapi.Chattable, error) {
	if message.Sticker == nil {
		return nil, nil
	}

	if sr.r.Float32() > sr.cfg.ReactChance {
		return nil, nil
	}

	randomIndex := sr.r.Intn(len(sr.stickers))
	sticker := sr.stickers[randomIndex]

	msg := tgbotapi.NewStickerShare(message.Chat.ID, sticker)
	msg.ReplyToMessageID = message.MessageID

	return msg, nil
}
