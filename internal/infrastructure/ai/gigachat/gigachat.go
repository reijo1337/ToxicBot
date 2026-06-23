package gigachat

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"sync"
	"time"

	"github.com/reijo1337/ToxicBot/pkg/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const (
	defaultAuthURL = "https://ngw.devices.sberbank.ru:9443/api/v2/oauth"
	defaultBaseURL = "https://gigachat.devices.sberbank.ru/api/v1"

	maxRetries = 3
)

type Client struct {
	httpClient *http.Client
	cfg        config
	authURL    string
	baseURL    string

	mu      sync.Mutex
	token   string
	tokenEx time.Time
}

// OAuth token response.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresAt   int64  `json:"expires_at"` // unix millis
}

// File upload response.
type uploadResponse struct {
	ID string `json:"id"`
}

// Chat completions request/response.
type chatRequest struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
}

type message struct {
	Role        string   `json:"role"`
	Content     string   `json:"content"`
	Attachments []string `json:"attachments,omitempty"`
}

type chatResponse struct {
	Choices []choice `json:"choices"`
}

type choice struct {
	Message responseMessage `json:"message"`
}

type responseMessage struct {
	Content string `json:"content"`
}

func New() (*Client, error) {
	client := &Client{
		authURL: defaultAuthURL,
		baseURL: defaultBaseURL,
	}

	if err := client.parseConfig(); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	client.httpClient = &http.Client{
		Timeout: client.cfg.Timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion:         tls.VersionTLS12,
				InsecureSkipVerify: true, //nolint:gosec // GigaChat uses self-signed certs
			},
		},
	}

	return client, nil
}

func (c *Client) GenerateContent(
	ctx context.Context,
	prompt string,
	imageBytes []byte,
) (string, error) {
	ctx, span := tracing.Tracer().Start(ctx, "gen_ai gigachat")
	defer span.End()
	span.SetAttributes(
		attribute.String("gen_ai.system", "gigachat"),
		attribute.String("gen_ai.request.model", c.cfg.Model),
		attribute.Int("gen_ai.input.image_bytes", len(imageBytes)),
		tracing.ContentAttr("gen_ai.input", prompt),
	)

	out, err := c.generateContentWithBaseURL(ctx, prompt, imageBytes, c.baseURL)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "gigachat request failed")
		return "", err
	}
	span.SetAttributes(tracing.ContentAttr("gen_ai.output", out))
	return out, nil
}

func (c *Client) generateContentWithBaseURL(
	ctx context.Context,
	prompt string,
	imageBytes []byte,
	apiBaseURL string,
) (string, error) {
	token, err := c.getToken(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get token: %w", err)
	}

	fileID, err := c.uploadImageWithBaseURL(ctx, token, imageBytes, apiBaseURL)
	if err != nil {
		return "", fmt.Errorf("failed to upload image: %w", err)
	}

	return c.chatWithBaseURL(ctx, token, prompt, fileID, apiBaseURL)
}

func (c *Client) getToken(ctx context.Context) (string, error) {
	return c.getTokenWithURL(ctx, c.authURL)
}

func (c *Client) getTokenWithURL(ctx context.Context, tokenURL string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && time.Now().Before(c.tokenEx) {
		return c.token, nil
	}

	rqUID, err := newUUID()
	if err != nil {
		return "", fmt.Errorf("failed to generate RqUID: %w", err)
	}

	body := []byte("scope=" + c.cfg.Scope)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create auth request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("RqUID", rqUID) //nolint
	req.Header.Set("Authorization", "Basic "+c.cfg.AuthKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("auth request failed: %w", err)
	}
	defer resp.Body.Close() //nolint

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read auth response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("auth error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var tokenResp tokenResponse
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal auth response: %w", err)
	}

	c.token = tokenResp.AccessToken
	// expires_at в миллисекундах, берём с запасом в 1 минуту
	c.tokenEx = time.UnixMilli(tokenResp.ExpiresAt).Add(-time.Minute)

	return c.token, nil
}

func (c *Client) uploadImageWithBaseURL(
	ctx context.Context,
	token string,
	imageBytes []byte,
	apiBaseURL string,
) (string, error) {
	mimeType := http.DetectContentType(imageBytes)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="image.jpg"`)
	header.Set("Content-Type", mimeType)

	part, err := writer.CreatePart(header)
	if err != nil {
		return "", fmt.Errorf("failed to create form file: %w", err)
	}

	if _, err := part.Write(imageBytes); err != nil {
		return "", fmt.Errorf("failed to write image data: %w", err)
	}

	if err := writer.WriteField("purpose", "general"); err != nil {
		return "", fmt.Errorf("failed to write purpose field: %w", err)
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("failed to close multipart writer: %w", err)
	}

	url := apiBaseURL + "/files"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return "", fmt.Errorf("failed to create upload request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close() //nolint

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read upload response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("upload error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var uploadResp uploadResponse
	if err := json.Unmarshal(respBody, &uploadResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal upload response: %w", err)
	}

	if uploadResp.ID == "" {
		return "", errors.New("empty file ID in upload response")
	}

	return uploadResp.ID, nil
}

func (c *Client) chatWithBaseURL(
	ctx context.Context,
	token, prompt, fileID, apiBaseURL string,
) (string, error) {
	chatReq := chatRequest{
		Model: c.cfg.Model,
		Messages: []message{
			{
				Role:        "user",
				Content:     prompt,
				Attachments: []string{fileID},
			},
		},
	}

	jsonData, err := json.Marshal(chatReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal chat request: %w", err)
	}

	url := apiBaseURL + "/chat/completions"

	var resp *http.Response

	for i := 0; i <= maxRetries; i++ {
		var httpReq *http.Request

		httpReq, err = http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			url,
			bytes.NewBuffer(jsonData),
		)
		if err != nil {
			return "", fmt.Errorf("failed to create chat request: %w", err)
		}

		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+token)

		resp, err = c.httpClient.Do(httpReq)
		if err == nil && resp.StatusCode < 500 {
			break
		}

		if i < maxRetries {
			if resp != nil {
				resp.Body.Close() //nolint
			}
			time.Sleep(time.Duration(i+1) * time.Second)
		}
	}

	if err != nil {
		return "", fmt.Errorf("failed to send chat request after retries: %w", err)
	}
	defer resp.Body.Close() //nolint

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read chat response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("chat API error (status %d): %s", resp.StatusCode, string(body))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal chat response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", errors.New("no choices in response")
	}

	content := chatResp.Choices[0].Message.Content
	if content == "" {
		return "", errors.New("empty content in response")
	}

	return content, nil
}

func newUUID() (string, error) {
	var uuid [16]byte
	if _, err := rand.Read(uuid[:]); err != nil {
		return "", err
	}

	uuid[6] = (uuid[6] & 0x0f) | 0x40 // version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // variant 10

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%12x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16]), nil
}
