// Package llm — OpenAI-совместимый клиент стенда: контракт продового deepseek-клиента
// (finish_reason != stop → ошибка), но с параметрами модели, адреса и полей тела.
package llm

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/reijo1337/ToxicBot/internal/features/message"
)

// Client запоминает последнюю ошибку: Generator ошибки LLM глотает и уходит в фоллбэк.
// Один Client на пару вариант × модель, вызовы внутри пары последовательные.
type Client struct {
	sdk         openai.Client
	model       string
	temperature float64
	maxTokens   int64
	extraBody   map[string]any

	mu      sync.Mutex
	lastErr error
}

type Params struct {
	BaseURL     string
	APIKey      string
	Model       string
	Temperature float64
	MaxTokens   int64
	ExtraBody   map[string]any
}

func New(p Params) *Client {
	return &Client{
		sdk: openai.NewClient(
			option.WithAPIKey(p.APIKey),
			option.WithBaseURL(p.BaseURL),
			option.WithMaxRetries(2),
		),
		model:       p.Model,
		temperature: p.Temperature,
		maxTokens:   p.MaxTokens,
		extraBody:   p.ExtraBody,
	}
}

func (c *Client) Chat(ctx context.Context, msgs ...message.LLMMessage) (string, error) {
	out, err := c.chat(ctx, msgs)
	c.mu.Lock()
	c.lastErr = err
	c.mu.Unlock()
	return out, err
}

// TakeErr отдаёт ошибку последнего вызова и сбрасывает её.
func (c *Client) TakeErr() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	err := c.lastErr
	c.lastErr = nil
	return err
}

func (c *Client) chat(ctx context.Context, msgs []message.LLMMessage) (string, error) {
	if len(msgs) == 0 {
		return "", errors.New("пустой envelope")
	}

	opts := make([]option.RequestOption, 0, len(c.extraBody))
	for k, v := range c.extraBody {
		opts = append(opts, option.WithJSONSet(k, v))
	}

	resp, err := c.sdk.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model:       c.model,
		Messages:    toSDKMessages(msgs),
		MaxTokens:   openai.Int(c.maxTokens),
		Temperature: openai.Float(c.temperature),
	}, opts...)
	if err != nil {
		return "", fmt.Errorf("chat: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("пустой choices")
	}

	choice := resp.Choices[0]
	switch choice.FinishReason {
	case "", "stop", "tool_calls":
		return choice.Message.Content, nil
	default:
		return "", fmt.Errorf(
			"finish_reason=%q completion_tokens=%d reasoning_tokens=%d content_len=%d",
			choice.FinishReason,
			resp.Usage.CompletionTokens,
			resp.Usage.CompletionTokensDetails.ReasoningTokens,
			len(choice.Message.Content),
		)
	}
}

func toSDKMessages(in []message.LLMMessage) []openai.ChatCompletionMessageParamUnion {
	out := make([]openai.ChatCompletionMessageParamUnion, 0, len(in))
	for _, m := range in {
		switch m.Role {
		case message.RoleSystem:
			out = append(out, openai.ChatCompletionMessageParamUnion{
				OfSystem: &openai.ChatCompletionSystemMessageParam{
					Content: openai.ChatCompletionSystemMessageParamContentUnion{
						OfString: openai.String(m.Content),
					},
				},
			})
		case message.RoleUser:
			user := &openai.ChatCompletionUserMessageParam{
				Content: openai.ChatCompletionUserMessageParamContentUnion{
					OfString: openai.String(m.Content),
				},
			}
			if m.Name != "" {
				user.Name = openai.String(m.Name)
			}
			out = append(out, openai.ChatCompletionMessageParamUnion{OfUser: user})
		case message.RoleAssistant:
			ass := &openai.ChatCompletionAssistantMessageParam{
				Content: openai.ChatCompletionAssistantMessageParamContentUnion{
					OfString: openai.String(m.Content),
				},
			}
			if m.Name != "" {
				ass.Name = openai.String(m.Name)
			}
			out = append(out, openai.ChatCompletionMessageParamUnion{OfAssistant: ass})
		}
	}
	return out
}
