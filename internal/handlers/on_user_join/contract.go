package on_user_join

//go:generate go tool go.uber.org/mock/mockgen -source $GOFILE -destination mocks_test.go -package ${GOPACKAGE}
import "context"

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
