package on_user_join

import (
	"context"
	"time"
)

func (g *Greetings) reloadMessages() error {
	r, err := g.storage.GetEnabledGreetings()
	if err != nil {
		return err
	}

	m := make([]string, len(r))
	copy(m, r)

	g.muMsg.Lock()
	defer g.muMsg.Unlock()
	g.messages = m

	return nil
}

func (g *Greetings) runUpdater(ctx context.Context) {
	t := time.NewTimer(g.updateMessagesPeriod)

	for {
		select {
		case <-t.C:
			if err := g.reloadMessages(); err != nil {
				g.logger.Warn(
					g.logger.WithError(
						g.logger.WithField(
							ctx,
							"handler", "greetings",
						),
						err,
					),
					"cannot reload messages",
				)
			}
		case <-ctx.Done():
			return
		}
	}
}
