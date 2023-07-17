//go:generate mockgen -source $GOFILE -destination mocks_test.go -package ${GOPACKAGE}
package message

import "context"

type messageRepository interface {
	GetEnabledRandom() ([]string, error)
}

type logger interface {
	WithError(context.Context, error) context.Context
	Warn(context.Context, string)
}

type randomizer interface {
	Float32() float32
	Intn(n int) int
}
