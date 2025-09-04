//go:generate go tool go.uber.org/mock/mockgen -source $GOFILE -destination mocks_test.go -package ${GOPACKAGE}
package on_voice

import (
	"context"

	"github.com/reijo1337/ToxicBot/internal/features/stats"
	"gopkg.in/telebot.v3"
)

type voicesRepository interface {
	GetEnabledVoices() ([]string, error)
}

type logger interface {
	WithError(context.Context, error) context.Context
	WithField(context.Context, string, any) context.Context
	Warn(context.Context, string)
	Error(context.Context, string)
}

type randomizer interface {
	Float32() float32
	Intn(n int) int
}

type downloader interface {
	FileByID(fileID string) (telebot.File, error)
}

type statIncer interface {
	Inc(ctx context.Context, chatID, userID int64, op stats.OperationType, opts ...stats.Option)
}
