package app

import "github.com/kelseyhightower/envconfig"

type config struct {
	TelegramToken string `envconfig:"TELEGRAM_TOKEN" required:"true"`
}

func (a *Application) parseConfig() error {
	if err := envconfig.Process("", &a.cfg); err != nil {
		envconfig.Usage("", a.cfg)
		return err
	}

	return nil
}
