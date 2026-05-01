package deepseek

import (
	"context"
	"errors"
	"fmt"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/reijo1337/ToxicBot/internal/features/message"
)

// Client is a thin DeepSeek wrapper on top of the official OpenAI Go SDK.
// DeepSeek exposes an OpenAI-compatible Chat Completions endpoint, so we
// reuse the SDK by pointing it at https://api.deepseek.com/v1.
type Client struct {
	sdk   openai.Client
	model string
}

const defaultModel = "deepseek-v4-flash"

func New() (*Client, error) {
	cfg, err := parseConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	sdk := openai.NewClient(
		option.WithAPIKey(cfg.APIKey),
		option.WithBaseURL(cfg.BaseURL),
		option.WithRequestTimeout(cfg.Timeout),
		option.WithMaxRetries(cfg.MaxRetries),
	)
	return &Client{sdk: sdk, model: defaultModel}, nil
}

// Chat sends the prepared message envelope to DeepSeek and returns the
// assistant content of the first choice. LLMMessage.Name is mapped to the
// OpenAI `messages[].name` field for user and assistant messages.
func (c *Client) Chat(
	ctx context.Context,
	msgs ...message.LLMMessage,
) (string, error) {
	if len(msgs) == 0 {
		return "", errors.New("no messages provided")
	}

	resp, err := c.sdk.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model:    c.model,
		Messages: toSDKMessages(msgs),
	})
	if err != nil {
		return "", fmt.Errorf("deepseek chat: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("no choices in response")
	}
	return resp.Choices[0].Message.Content, nil
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
