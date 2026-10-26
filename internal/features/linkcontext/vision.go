package linkcontext

import (
	"context"
	"errors"
)

var (
	errVisionLimited = errors.New("vision request limit reached")
	errVisionBusy    = errors.New("vision concurrency limit reached")
)

// Vision applies the shared process budgets without changing the caller's prompt or deadline.
type Vision struct {
	client      imageDescriber
	limits      *Limits
	waitForSlot bool
}

func NewVision(client imageDescriber, limits *Limits) *Vision {
	if limits == nil {
		limits = NewLimits()
	}
	return &Vision{client: client, limits: limits}
}

func (v *Vision) GenerateContent(ctx context.Context, prompt string, data []byte) (string, error) {
	if err := v.acquireSlot(ctx); err != nil {
		return "", err
	}
	defer func() { <-v.limits.visionSlots }()
	if !v.limits.allowVision() {
		return "", errVisionLimited
	}
	return v.client.GenerateContent(ctx, prompt, data)
}

func newPreviewVision(client imageDescriber, limits *Limits) *Vision {
	vision := NewVision(client, limits)
	vision.waitForSlot = true
	return vision
}

func (v *Vision) acquireSlot(ctx context.Context) error {
	if !v.waitForSlot {
		select {
		case v.limits.visionSlots <- struct{}{}:
			return nil
		default:
			return errVisionBusy
		}
	}
	select {
	case v.limits.visionSlots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
