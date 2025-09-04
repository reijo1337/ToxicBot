package stat

import (
	"context"
	"time"

	"github.com/reijo1337/ToxicBot/internal/features/stats"
)

type storage interface {
	GetTotalStat(ctx context.Context) (*stats.TotalStat, error)
	GetDetailedStat(ctx context.Context, date time.Time) ([]stats.DetailedStat, error)
}
