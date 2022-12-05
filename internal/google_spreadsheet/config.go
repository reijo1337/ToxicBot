package google_spreadsheet

import (
	"time"

	"github.com/kelseyhightower/envconfig"
)

type config struct {
	SpreadsheetID string        `envconfig:"GOOGLE_SPREADSHEET_ID" required:"true"`
	Credentials   string        `envconfig:"GOOGLE_CREDENTIALS" required:"true"`
	CacheInterval time.Duration `envconfig:"GOOGLE_CACHE_INTERVAL" default:"15m"`
}

func (c *Client) parseConfig() error {
	if err := envconfig.Process("", &c.cfg); err != nil {
		return err
	}

	return nil
}
