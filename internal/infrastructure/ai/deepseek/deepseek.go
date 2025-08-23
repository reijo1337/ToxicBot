package deepseek

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Role string

const (
	RoleUser   Role = "user"
	RoleSystem Role = "system"
)

type Client struct {
	httpClient *http.Client
	cfg        config
}

// ChatMessage представляет сообщение в чате
type ChatMessage struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

// ChatRequest представляет запрос к API чата
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
}

// ChatResponse представляет ответ от API чата
type ChatResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// ErrorResponse представляет ошибку от API
type ErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code,omitempty"`
	} `json:"error"`
}

// New создает новый клиент Deepseek
func New() (*Client, error) {
	client := &Client{}

	if err := client.parseConfig(); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	client.httpClient = &http.Client{
		Timeout: client.cfg.Timeout,
	}

	return client, nil
}

// Chat отправляет запрос к API чата
func (c *Client) Chat(ctx context.Context, msgs ...ChatMessage) (string, error) {
	if len(msgs) == 0 {
		return "", errors.New("no messages provided")
	}

	req := chatRequest{
		Model:    "deepseek-chat",
		Messages: msgs,
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.cfg.BaseURL+"/v1/chat/completions",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	var resp *http.Response
	for i := 0; i <= c.cfg.MaxRetries; i++ {
		resp, err = c.httpClient.Do(httpReq)
		if err == nil && resp.StatusCode < 500 {
			break
		}

		if i < c.cfg.MaxRetries {
			time.Sleep(time.Duration(i+1) * time.Second)
		}
	}

	if err != nil {
		return "", fmt.Errorf("failed to send request after retries: %w", err)
	}
	defer resp.Body.Close() //nolint

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errorResp ErrorResponse
		if err := json.Unmarshal(body, &errorResp); err != nil {
			return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
		}
		return "", fmt.Errorf("API error: %s", errorResp.Error.Message)
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", errors.New("no choices in response")
	}

	return chatResp.Choices[0].Message.Content, nil
}
