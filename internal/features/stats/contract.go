package stats

import (
	"context"
	"time"
)

type storage interface {
	Create(ctx context.Context, event Response) error
	GetTotalStat(ctx context.Context) (*TotalStat, error)
	GetDetailedStat(ctx context.Context, date time.Time) ([]DetailedStat, error)
}

type logger interface {
	WithError(context.Context, error) context.Context
	WithFields(context.Context, map[string]any) context.Context
	Warn(context.Context, string)
}
