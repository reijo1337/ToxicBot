package on_sticker

import (
	"github.com/kelseyhightower/envconfig"
)

type config struct {
	FilePath    string  `envconfig:"STICKERS_FILE" default:"data/stickers"`
	ReactChance float32 `envconfig:"STICKER_REACTIONS_CHANCE" default:"0.4"`
}

func (sr *StickerReactions) parseConfig() error {
	if err := envconfig.Process("", &sr.cfg); err != nil {
		envconfig.Usage("", sr.cfg)
		return err
	}

	return nil
}
