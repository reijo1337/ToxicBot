//go:generate go tool go.uber.org/mock/mockgen -source $GOFILE -destination mocks_test.go -package ${GOPACKAGE}
package tagger

import (
	"context"

	"github.com/reijo1337/ToxicBot/internal/features/chatsettings"
	"github.com/reijo1337/ToxicBot/internal/features/message"
	"github.com/reijo1337/ToxicBot/internal/features/stats"
)

type nicknameRepository interface {
	GetEnabledNicknames() ([]string, error)
}

type messageGenerator interface {
	GetMessageText(ctx context.Context, prompt string, aiChance float32) message.GenerationResult
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

type statIncer interface {
	Inc(ctx context.Context, chatID, userID int64, op stats.OperationType, opts ...stats.Option)
}

type settingsProvider interface {
	GetForChat(ctx context.Context, chatID int64) (*chatsettings.Settings, error)
}
