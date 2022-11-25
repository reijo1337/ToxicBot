package google_spreadsheet

import (
	"context"
	"fmt"
	"net/http"

	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"
	"gopkg.in/Iwark/spreadsheet.v2"
)

type Client struct {
	configJwt *jwt.Config
	client    *http.Client
	service   *spreadsheet.Service
	cfg       config
}

func New(ctx context.Context) (googleClient *Client, err error) {
	googleClient = &Client{}
	if err = googleClient.parseConfig(); err != nil {
		return googleClient, fmt.Errorf("cannot parse config: %w", err)
	}

	googleClient.configJwt, err = google.JWTConfigFromJSON(googleClient.cfg.Credentials, spreadsheet.Scope)
	if err != nil {
		return googleClient, fmt.Errorf("failed to create config for google jwt: %w", err)
	}

	googleClient.client = googleClient.configJwt.Client(ctx)
	googleClient.service = spreadsheet.NewServiceWithClient(googleClient.client)
	return googleClient, err
}

func (c *Client) GetSpreadsheet() (spreadsheet.Spreadsheet, error) {
	return c.service.FetchSpreadsheet(c.cfg.SpreadsheetID, spreadsheet.WithCache(c.cfg.CacheInterval))
}
