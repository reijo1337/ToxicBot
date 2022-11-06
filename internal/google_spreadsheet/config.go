package google_spreadsheet

import (
	"github.com/kelseyhightower/envconfig"
	"time"
)

type config struct {
	Credentials   credentialType `envconfig:"GOOGLE_CREDENTIALS" required:"true"`
	CacheInterval time.Duration  `envconfig:"GOOGLE_CACHE_INTERVAL" default:"15m"`
	SpreadsheetID string         `envconfig:"GOOGLE_SPREADSHEET_ID" required:"true"`
}

type credentialType []byte

func (c *credentialType) UnmarshalBinary(data []byte) error {
	*c = data
	return nil
}

func (c *Client) parseConfig() error {
	if err := envconfig.Process("", &c.cfg); err != nil {
		return err
	}

	return nil
}
