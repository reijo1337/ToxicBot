package on_voice

import (
	"context"
	"fmt"
	"time"
)

func (h *Handler) reloadVoices() error {
	r, err := h.storage.GetEnabledVoices()
	if err != nil {
		return fmt.Errorf("storage.GetEnabledVoices error: %w", err)
	}

	voices := make([]string, len(r))

	copy(voices, r)

	h.muVcs.Lock()
	defer h.muVcs.Unlock()
	h.voices = voices

	return nil
}

func (h *Handler) runUpdater(ctx context.Context) {
	t := time.NewTimer(h.updatePeriod)

	for {
		select {
		case <-t.C:
			if err := h.reloadVoices(); err != nil {
				h.logger.Warn(h.logger.WithError(ctx, err), "cannot reload voices")
			}
		case <-ctx.Done():
			return
		}
	}
}
