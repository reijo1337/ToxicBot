package on_sticker

import (
	"context"
	"time"
)

func (sr *StickerReactions) reloadStickers() error {
	r, err := sr.storage.GetEnabledStickers()
	if err != nil {
		return err
	}

	stickers := make([]string, len(r))
	copy(stickers, r)

	sr.muStk.Lock()
	defer sr.muStk.Unlock()
	sr.stickers = stickers

	return nil
}

func (sr *StickerReactions) runUpdater(ctx context.Context) {
	t := time.NewTimer(sr.updateStickersPeriod)

	for {
		select {
		case <-t.C:
			if err := sr.reloadStickers(); err != nil {
				sr.logger.Warn(sr.logger.WithError(ctx, err), "cannot reload stickers")
			}
		case <-ctx.Done():
			return
		}
	}
}
