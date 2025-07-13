//go:generate go tool go.uber.org/mock/mockgen -source $GOFILE -destination mocks_test.go -package ${GOPACKAGE}
package tagger

import "context"

type nicknameRepository interface {
	GetEnabledNicknames() ([]string, error)
}

type messageGenerator interface {
	GetMessageText() string
}

type logger interface {
	WithError(context.Context, error) context.Context
	WithFields(context.Context, map[string]any) context.Context
	Warn(context.Context, string)
}

type randomizer interface {
	Intn(int) int
	Int63n(int64) int64
}
