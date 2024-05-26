package on_voice

import (
	"context"
	"fmt"
	"time"

	"gopkg.in/telebot.v3"
)

func (h *Handler) reloadVoices() error {
	r, err := h.storage.GetEnabledVoices()
	if err != nil {
		return err
	}

	voices := make([]telebot.File, len(r))

	for i, id := range r {
		file, err := h.downloader.FileByID(id)
		if err != nil {
			return fmt.Errorf("can't get file %s: %w", id, err)
		}
		voices[i] = file
	}

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
				h.logger.Warn(
					h.logger.WithError(
						context.Background(),
						err,
					),
					"cannot reload voices",
				)
			}
		case <-ctx.Done():
			return
		}
	}
}
