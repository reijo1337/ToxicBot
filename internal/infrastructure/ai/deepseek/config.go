package deepseek

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

type config struct {
	APIKey     string        `envconfig:"DEEPSEEK_API_KEY"     required:"true"`
	BaseURL    string        `envconfig:"DEEPSEEK_BASE_URL"                    default:"https://api.deepseek.com/v1"`
	Timeout    time.Duration `envconfig:"DEEPSEEK_TIMEOUT"                     default:"30s"`
	MaxRetries int           `envconfig:"DEEPSEEK_MAX_RETRIES"                 default:"3"`
}

func parseConfig() (config, error) {
	var c config
	if err := envconfig.Process("", &c); err != nil {
		return c, fmt.Errorf("envconfig.Process error: %w", err)
	}
	return c, nil
}
