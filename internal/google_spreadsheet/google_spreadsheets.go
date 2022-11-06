package google_spreadsheet

import (
	"context"
	"fmt"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"
	"gopkg.in/Iwark/spreadsheet.v2"
	"net/http"
)

type Client struct {
	cfg       config
	configJwt *jwt.Config
	client    *http.Client
	service   *spreadsheet.Service
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
