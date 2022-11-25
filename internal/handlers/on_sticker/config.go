package on_sticker

import (
	"time"

	"github.com/kelseyhightower/envconfig"
)

type config struct {
	ReactChance          float32       `envconfig:"STICKER_REACTIONS_CHANCE" default:"0.4"`
	UpdateStickersPeriod time.Duration `envconfig:"STICKERS_UPDATE_PERIOD" default:"30m"`
}

func (sr *StickerReactions) parseConfig() error {
	if err := envconfig.Process("", &sr.cfg); err != nil {
		if err = envconfig.Usage("", sr.cfg); err != nil {
			return err
		}
	}

	return nil
}
