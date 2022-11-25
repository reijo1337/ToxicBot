package on_sticker

import (
	"context"
	"time"
)

func (sr *StickerReactions) reloadStickers() error {
	r, err := sr.storage.GetStickers()
	if err != nil {
		return err
	}

	r = r.GetEnabled()

	stickers := make([]string, 0, len(r))
	for _, dto := range r {
		stickers = append(stickers, dto.StickerID)
	}

	sr.muStk.Lock()
	defer sr.muStk.Unlock()
	sr.stickers = stickers

	return nil
}

func (sr *StickerReactions) runUpdater(ctx context.Context) {
	t := time.NewTimer(sr.cfg.UpdateStickersPeriod)
	for {
		select {
		case <-t.C:
			if err := sr.reloadStickers(); err != nil {
				sr.logger.WithError(err).Warn("cannot reload stickers")
			}
		case <-ctx.Done():
			return
		}
	}
}
