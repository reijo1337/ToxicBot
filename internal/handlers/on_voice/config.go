package on_voice

import (
	"time"

	"github.com/kelseyhightower/envconfig"
)

type config struct {
	ReactChance        float32       `envconfig:"VOICE_REACTIONS_CHANCE" default:"0.4"`
	UpdateVoicesPeriod time.Duration `envconfig:"VOICE_UPDATE_PERIOD" default:"30m"`
}

func (h *Handler) parseConfig() error {
	if err := envconfig.Process("", &h.cfg); err != nil {
		if err = envconfig.Usage("", h.cfg); err != nil {
			return err
		}
	}

	return nil
}
