package on_user_join

import (
	"time"

	"github.com/kelseyhightower/envconfig"
)

type config struct {
	UpdateMessagesPeriod time.Duration `envconfig:"ON_USER_JOIN_UPDATE_MESSAGES_PERIOD" default:"10m"`
}

func (g *Greetings) parseConfig() error {
	if err := envconfig.Process("", &g.cfg); err != nil {
		envconfig.Usage("", g.cfg)
		return err
	}

	return nil
}
