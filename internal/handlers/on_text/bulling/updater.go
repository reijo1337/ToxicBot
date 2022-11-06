package bulling

import (
	"context"
	"github.com/mb-14/gomarkov"
	"strings"
	"time"
)

func (b *bulling) reloadMessages() error {
	r, err := b.storage.GetRandom()
	if err != nil {
		return err
	}

	r = r.GetEnabled()

	m := make([]string, 0, len(r))
	for _, dto := range r {
		m = append(m, dto.Text)
	}

	chain := gomarkov.NewChain(1)
	for _, message := range m {
		chain.Add(strings.Split(strings.Trim(message, " "), " "))
	}

	b.muMsg.Lock()
	defer b.muMsg.Unlock()
	b.messages = m
	b.chain = chain

	return nil
}

func (b *bulling) runUpdater(ctx context.Context) {
	t := time.NewTimer(b.cfg.UpdateMessagesPeriod)
	for {
		select {
		case <-t.C:
			if err := b.reloadMessages(); err != nil {
				b.logger.WithError(err).Warn("cannot reload messages")
			}
		case <-ctx.Done():
			return
		}
	}
}
