//go:generate mockgen -source $GOFILE -destination mocks_test.go -package ${GOPACKAGE}
package on_voice

import (
	"context"

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
