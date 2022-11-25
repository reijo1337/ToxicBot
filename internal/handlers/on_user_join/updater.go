package on_user_join

import (
	"context"
	"time"
)

func (g *Greetings) reloadMessages() error {
	r, err := g.storage.GetGreetings()
	if err != nil {
		return err
	}

	r = r.GetEnabled()

	m := make([]string, 0, len(r))
	for _, dto := range r {
		m = append(m, dto.Text)
	}

	g.muMsg.Lock()
	defer g.muMsg.Unlock()
	g.messages = m

	return nil
}

func (g *Greetings) runUpdater(ctx context.Context) {
	t := time.NewTimer(g.cfg.UpdateMessagesPeriod)
	for {
		select {
		case <-t.C:
			if err := g.reloadMessages(); err != nil {
				g.logger.WithError(err).Warn("cannot reload messages")
			}
		case <-ctx.Done():
			return
		}
	}
}
