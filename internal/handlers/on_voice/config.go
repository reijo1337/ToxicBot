package on_voice

import (
	"github.com/kelseyhightower/envconfig"
)

type config struct {
	FilePath    string  `envconfig:"VOICE_FILE" default:"data/voice"`
	ReactChance float32 `envconfig:"VOICE_REACTIONS_CHANCE" default:"0.25"`
}

func (h *Handler) parseConfig() error {
	if err := envconfig.Process("", &h.cfg); err != nil {
		envconfig.Usage("", h.cfg)
		return err
	}

	return nil
}
