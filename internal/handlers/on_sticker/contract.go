//go:generate go tool go.uber.org/mock/mockgen -source $GOFILE -destination mocks_test.go -package ${GOPACKAGE}
package on_sticker

import (
	"context"

	"github.com/reijo1337/ToxicBot/internal/features/stats"
)

type stickerRepository interface {
	GetEnabledStickers() ([]string, error)
}

type logger interface {
	WithError(context.Context, error) context.Context
	Warn(context.Context, string)
}

type randomizer interface {
	Float32() float32
	Intn(n int) int
}

type statIncer interface {
	Inc(ctx context.Context, chatID, userID int64, op stats.OperationType, opts ...stats.Option)
}
