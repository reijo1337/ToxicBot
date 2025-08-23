package deepseek

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

type config struct {
	APIKey     string        `envconfig:"DEEPSEEK_API_KEY" required:"true"`
	BaseURL    string        `envconfig:"DEEPSEEK_BASE_URL" default:"https://api.deepseek.com"`
	Timeout    time.Duration `envconfig:"DEEPSEEK_TIMEOUT" default:"30s"`
	MaxRetries int           `envconfig:"DEEPSEEK_MAX_RETRIES" default:"3"`
}

func (c *Client) parseConfig() error {
	if err := envconfig.Process("", &c.cfg); err != nil {
		return fmt.Errorf("envconfig.Process error: %w", err)
	}

	return nil
}

