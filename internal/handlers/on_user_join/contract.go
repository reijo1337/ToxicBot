package on_user_join

//go:generate go tool go.uber.org/mock/mockgen -source $GOFILE -destination mocks_test.go -package ${GOPACKAGE}
import (
	"context"

	"github.com/reijo1337/ToxicBot/internal/features/stats"
)

type greetingsRepository interface {
	GetEnabledGreetings() ([]string, error)
}

type logger interface {
	WithError(context.Context, error) context.Context
	WithField(context.Context, string, any) context.Context
	Warn(context.Context, string)
}

type randomizer interface {
	Intn(n int) int
}

type statIncer interface {
	Inc(ctx context.Context, chatID, userID int64, op stats.OperationType, opts ...stats.Option)
}
