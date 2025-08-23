package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	TelegramToken                string        `envconfig:"TELEGRAM_TOKEN"                      required:"true"`
	StickerSets                  []string      `envconfig:"STICKER_SETS"                        default:"static_bulling_by_stickersthiefbot"`
	TaggerIntervalFrom           time.Duration `envconfig:"TAGGER_INTERVAL_FROM"                default:"10h"`
	TelegramLongPollTimeout      time.Duration `envconfig:"TELEGRAM_LONG_POLL_TIMEOUT"          default:"10s"`
	ThresholdCount               int           `envconfig:"BULLINGS_THRESHOLD_COUNT"            default:"5"`
	ThresholdTime                time.Duration `envconfig:"BULLINGS_THRESHOLD_TIME"             default:"1m"`
	Cooldown                     time.Duration `envconfig:"BULLINGS_COOLDOWN"                   default:"1h"`
	UpdateStickersPeriod         time.Duration `envconfig:"STICKERS_UPDATE_PERIOD"              default:"30m"`
	UpdateMessagesPeriod         time.Duration `envconfig:"ON_USER_JOIN_UPDATE_MESSAGES_PERIOD" default:"10m"`
	TaggerIntervalTo             time.Duration `envconfig:"TAGGER_INTERVAL_TO"                  default:"24h"`
	UpdateVoicesPeriod           time.Duration `envconfig:"VOICE_UPDATE_PERIOD"                 default:"30m"`
	BullingsUpdateMessagesPeriod time.Duration `envconfig:"BULLINGS_UPDATE_MESSAGES_PERIOD"     default:"10m"`
	StickerReactChance           float32       `envconfig:"STICKER_REACTIONS_CHANCE"            default:"0.4"`
	BullingsMarkovChance         float32       `envconfig:"BULLINGS_MARKOV_CHANCE"              default:"0.75"`
	BullingsAIChance             float32       `envconfig:"BULLINGS_AI_CHANCE"                  default:"0.75"`
	VoiceReactChance             float32       `envconfig:"VOICE_REACTIONS_CHANCE"              default:"0.8"`
	NicknamesUpdatePerios        time.Duration `envconfig:"NICKNAMES_UPDATE_PERIOD"             default:"10m"`
}

func Parse() (*Config, error) {
	var out Config
	if err := envconfig.Process("", &out); err != nil {
		if err = envconfig.Usage("", out); err != nil {
			return nil, fmt.Errorf("can't parse config: %w", err)
		}
	}

	if out.TaggerIntervalFrom > out.TaggerIntervalTo {
		return nil, errors.New("TAGGER_INTERVAL_FROM must be less or equal to TAGGER_INTERVAL_TO")
	}

	return &out, nil
}
