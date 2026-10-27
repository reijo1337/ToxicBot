package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/reijo1337/ToxicBot/pkg/logger"
	"gopkg.in/telebot.v3"
)

const (
	defaultRetryDelay   = time.Second
	maxRetryDelay       = 30 * time.Second
	maxReactionAttempts = 3
	maxBackoffAttempts  = 6
)

type reactionHandler interface {
	Handle(ctx context.Context, updateID int, event *telebot.MessageReaction) error
}

type Poller struct {
	contextMu        sync.RWMutex
	pollContext      context.Context
	reactions        reactionHandler
	log              *logger.Logger
	timeout          time.Duration
	offset           int
	requestAttempts  int
	reactionID       int
	reactionAttempts int
}

var _ telebot.Poller = (*Poller)(nil)

// updateEnvelope suppresses an SDK field whose type does not match Telegram's
// reaction-count array. Unsubscribed anonymous counters are acknowledged only.
type updateEnvelope struct {
	telebot.Update

	ReactionCount json.RawMessage `json:"message_reaction_count"`
	ordinary      bool
	malformed     bool
}

func NewPoller(timeout time.Duration, reactions reactionHandler, log *logger.Logger) *Poller {
	return &Poller{
		timeout:   timeout,
		reactions: reactions,
		log:       log,
		offset:    1,
	}
}

func (p *Poller) Poll(bot *telebot.Bot, dest chan telebot.Update, stop chan struct{}) {
	ctx, cancel := context.WithCancel(context.Background())
	p.contextMu.Lock()
	p.pollContext = ctx
	p.contextMu.Unlock()
	watchDone := make(chan struct{})
	defer func() {
		cancel()
		<-watchDone
		p.contextMu.Lock()
		p.pollContext = nil
		p.contextMu.Unlock()
	}()

	go func() {
		defer close(watchDone)

		select {
		case <-stop:
			cancel()
		case <-ctx.Done():
		}
	}()

	for {
		select {
		case <-stop:
			return
		default:
		}

		updates, err := p.getUpdates(bot)
		if err != nil {
			p.requestAttempts = min(p.requestAttempts+1, maxBackoffAttempts)
			if !p.retry(ctx, stop, err, "request", p.requestAttempts) {
				return
			}

			continue
		}

		p.requestAttempts = 0
		if !p.deliver(ctx, updates, dest, stop) {
			return
		}
	}
}

func (p *Poller) currentContext() context.Context {
	p.contextMu.RLock()
	defer p.contextMu.RUnlock()

	return p.pollContext
}

func (p *Poller) getUpdates(bot *telebot.Bot) ([]updateEnvelope, error) {
	params := struct {
		Offset         int      `json:"offset"`
		Timeout        int      `json:"timeout"`
		AllowedUpdates []string `json:"allowed_updates"`
	}{
		Offset:         p.offset,
		Timeout:        int(p.timeout / time.Second),
		AllowedUpdates: allowedUpdates(),
	}

	data, err := bot.Raw("getUpdates", params)
	if err != nil {
		return nil, err
	}

	var response struct {
		OK     bool              `json:"ok"`
		Result []json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}

	if !response.OK {
		return nil, errors.New("unsuccessful Telegram response")
	}

	updates := make([]updateEnvelope, 0, len(response.Result))

	for _, raw := range response.Result {
		update, err := decodeUpdate(raw)
		if err != nil {
			return nil, err
		}

		updates = append(updates, update)
	}

	return updates, nil
}

func allowedUpdates() []string {
	return []string{
		"message", "edited_message", "channel_post", "edited_channel_post",
		"inline_query", "chosen_inline_result", "callback_query",
		"shipping_query", "pre_checkout_query", "poll", "poll_answer",
		"my_chat_member", "chat_join_request", "message_reaction",
	}
}

func decodeUpdate(raw json.RawMessage) (updateEnvelope, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return updateEnvelope{}, errors.New("invalid Telegram update envelope")
	}

	var id int
	if err := json.Unmarshal(fields["update_id"], &id); err != nil || id <= 0 {
		return updateEnvelope{}, errors.New("missing valid Telegram update ID")
	}

	var update updateEnvelope
	if err := json.Unmarshal(raw, &update); err != nil {
		return updateEnvelope{
			Update: telebot.Update{
				ID: id,
			},
			malformed: true,
		}, nil
	}

	for name, value := range fields {
		if name != "message_reaction" && string(value) != "null" &&
			slices.Contains(allowedUpdates(), name) {
			update.ordinary = true
			break
		}
	}

	return update, nil
}

func (p *Poller) deliver(
	ctx context.Context,
	updates []updateEnvelope,
	dest chan telebot.Update,
	stop chan struct{},
) bool {
	for _, update := range updates {
		select {
		case <-stop:
			return false
		case <-ctx.Done():
			return false
		default:
		}

		if update.ID < p.offset {
			continue
		}

		switch {
		case update.malformed:
			p.log.Error(
				p.log.WithField(ctx, "update_id", update.ID),
				"skipping malformed Telegram update",
			)
		case update.MessageReaction != nil:
			if p.reactionID != update.ID {
				p.reactionID, p.reactionAttempts = update.ID, 0
			}
			p.reactionAttempts++
			if err := p.reactions.Handle(ctx, update.ID, update.MessageReaction); err != nil {
				logCtx := p.log.WithError(ctx, err)
				logCtx = p.log.WithField(logCtx, "update_id", update.ID)
				if update.MessageReaction.Chat != nil {
					logCtx = p.log.WithField(logCtx, "chat_id", update.MessageReaction.Chat.ID)
				}

				if p.reactionAttempts < maxReactionAttempts {
					return p.retry(logCtx, stop, err, "reaction", p.reactionAttempts)
				}

				p.log.Error(logCtx, "skipping Telegram reaction after repeated history errors")
			}
		case update.ordinary:
			select {
			case dest <- update.Update:
			case <-stop:
				return false
			case <-ctx.Done():
				return false
			}
		}

		p.offset = update.ID + 1
		p.reactionAttempts = 0
	}

	return true
}

func (p *Poller) retry(
	ctx context.Context,
	stop chan struct{},
	err error,
	stage string,
	attempt int,
) bool {
	delay := min(defaultRetryDelay<<min(max(attempt-1, 0), maxBackoffAttempts-1), maxRetryDelay)

	var flood telebot.FloodError
	if errors.As(err, &flood) {
		if requested := time.Duration(flood.RetryAfter) * time.Second; requested > delay {
			delay = requested
		}
	}

	select {
	case <-stop:
		return false
	default:
	}

	logCtx := p.log.WithFields(
		ctx,
		map[string]any{"stage": stage, "attempt": attempt, "delay": delay},
	)

	p.log.Warn(logCtx, "telegram update intake will retry")
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return true
	case <-stop:
		return false
	case <-ctx.Done():
		return false
	}
}
