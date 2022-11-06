package on_sticker

import (
	"github.com/kelseyhightower/envconfig"
	"time"
)

type config struct {
	ReactChance          float32       `envconfig:"STICKER_REACTIONS_CHANCE" default:"0.4"`
	UpdateStickersPeriod time.Duration `envconfig:"STICKERS_UPDATE_PERIOD" default:"30m"`
}

func (sr *StickerReactions) parseConfig() error {
	if err := envconfig.Process("", &sr.cfg); err != nil {
		envconfig.Usage("", sr.cfg)
		return err
	}

	return nil
}
