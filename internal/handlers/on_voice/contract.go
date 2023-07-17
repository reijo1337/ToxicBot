//go:generate mockgen -source $GOFILE -destination mocks_test.go -package ${GOPACKAGE}
package on_voice

import "context"

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
