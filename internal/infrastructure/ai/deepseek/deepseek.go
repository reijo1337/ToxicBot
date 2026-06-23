package deepseek

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/reijo1337/ToxicBot/internal/features/message"
	"github.com/reijo1337/ToxicBot/pkg/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// ErrResponseTruncated is returned when DeepSeek signals that the reply
// content is unusable: either cut off by max_tokens (`finish_reason:
// "length"`) or blocked by the safety filter (`finish_reason:
// "content_filter"`, which leaves Content empty or partial). In both cases
// shipping the body produces visible breakage, so callers should fall back
// to the list-based generator. The error name is historical — the contract
// is "content is not safe to ship", not specifically "truncated".
var ErrResponseTruncated = errors.New("deepseek response unusable")

// Client is a thin DeepSeek wrapper on top of the official OpenAI Go SDK.
// DeepSeek exposes an OpenAI-compatible Chat Completions endpoint, so we
// reuse the SDK by pointing it at https://api.deepseek.com/v1.
type Client struct {
	sdk         openai.Client
	model       string
	maxTokens   int64
	temperature float64
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
	return &Client{
		sdk:         sdk,
		model:       defaultModel,
		maxTokens:   cfg.MaxTokens,
		temperature: cfg.Temperature,
	}, nil
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

	ctx, span := tracing.Tracer().Start(ctx, "gen_ai deepseek")
	defer span.End()
	span.SetAttributes(
		attribute.String("gen_ai.system", "deepseek"),
		attribute.String("gen_ai.request.model", c.model),
	)
	// renderMessages joins the whole envelope (system + up to 100 history msgs);
	// skip that build when the span won't store it (tracing off / not sampled).
	if span.IsRecording() {
		span.SetAttributes(tracing.ContentAttr("gen_ai.input", renderMessages(msgs)))
	}

	resp, err := c.sdk.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model:       c.model,
		Messages:    toSDKMessages(msgs),
		MaxTokens:   openai.Int(c.maxTokens),
		Temperature: openai.Float(c.temperature),
	},
		// DeepSeek's V4 models run thinking mode ON by default, and the reasoning
		// tokens count against max_tokens. For a 1-3 sentence toxic reply the
		// chain-of-thought is pure overhead: it burned ~460 of the 500-token
		// budget, leaving the actual content truncated mid-word (finish_reason=
		// "length") so every call fell back to list-based phrases. Disable it via
		// the top-level `thinking` body field (DeepSeek extension, not in the SDK
		// struct) so the whole budget goes to the answer. See
		// https://api-docs.deepseek.com/api/create-chat-completion (Body > thinking).
		option.WithJSONSet("thinking", map[string]string{"type": "disabled"}),
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "deepseek request failed")
		return "", fmt.Errorf("deepseek chat: %w", err)
	}
	if len(resp.Choices) == 0 {
		err := errors.New("no choices in response")
		span.RecordError(err)
		span.SetStatus(codes.Error, "no choices")
		return "", err
	}

	choice := resp.Choices[0]
	span.SetAttributes(attribute.String("gen_ai.finish_reason", choice.FinishReason))

	switch choice.FinishReason {
	case "", "stop", "tool_calls":
		// Normal successful completions: empty (legacy DeepSeek responses
		// occasionally omit the field), "stop" (model finished naturally),
		// "tool_calls" (function-calling handoff — content is fine).
		if span.IsRecording() {
			span.SetAttributes(tracing.ContentAttr("gen_ai.output", choice.Message.Content))
		}
		return choice.Message.Content, nil
	default:
		// "length" — model hit max_tokens, content ends mid-thought / mid-word.
		// "content_filter" — safety filter wiped the body, content is empty
		// or partial. Any unknown future reason is treated the same way:
		// drop the content, surface the sentinel, let the caller fall back
		// to the list-based generator. No retry: both states are deterministic
		// for the same prompt — retrying would just burn the budget.
		//
		// Diagnostics: the three unusable states collapse into one sentinel, but
		// we fold the actual finish_reason and token usage into the error text so
		// prod logs can tell them apart without the model output — "length" with
		// high completion/reasoning tokens (model rambled past the cap) vs
		// "content_filter" with a near-empty body (safety layer wiped it) vs an
		// unexpected reason. errors.Is(ErrResponseTruncated) still holds via %w.
		err := fmt.Errorf(
			"finish_reason=%q completion_tokens=%d reasoning_tokens=%d content_len=%d: %w",
			choice.FinishReason,
			resp.Usage.CompletionTokens,
			resp.Usage.CompletionTokensDetails.ReasoningTokens,
			len(choice.Message.Content),
			ErrResponseTruncated,
		)
		span.RecordError(err)
		span.SetStatus(codes.Error, "response unusable")
		return "", err
	}
}

// renderMessages flattens the chat envelope into one string for the gen_ai.input
// span attribute: one "role[ name]: content" line per message.
func renderMessages(msgs []message.LLMMessage) string {
	var b strings.Builder
	for i, m := range msgs {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(string(m.Role))
		if m.Name != "" {
			b.WriteString(" ")
			b.WriteString(m.Name)
		}
		b.WriteString(": ")
		b.WriteString(m.Content)
	}
	return b.String()
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
