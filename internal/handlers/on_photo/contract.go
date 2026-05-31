//go:generate go tool go.uber.org/mock/mockgen -source $GOFILE -destination mocks_test.go -package ${GOPACKAGE}
package on_photo

import (
	"context"
	"io"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/reijo1337/ToxicBot/internal/features/chatsettings"
	"github.com/reijo1337/ToxicBot/internal/features/message"
	"github.com/reijo1337/ToxicBot/internal/features/stats"
	"gopkg.in/telebot.v3"
)

type imageDescriber interface {
	GenerateContent(ctx context.Context, prompt string, imageBytes []byte) (string, error)
}

type messageGenerator interface {
	GetMessageTextWithHistoryAndSteering(
		ctx context.Context,
		history []chathistory.Entry,
		aiChance float32,
		forceAI bool,
		steering string,
	) message.GenerationResult
}

type settingsProvider interface {
	GetForChat(ctx context.Context, chatID int64) (*chatsettings.Settings, error)
}

type historyBuffer interface {
	AddAll(chatID int64, entries ...chathistory.Entry)
	Get(chatID int64) []chathistory.Entry
}

type downloader interface {
	FileByID(fileID string) (telebot.File, error)
}

type fileReader interface {
	ReadFile(file *telebot.File) (io.ReadCloser, error)
}

type logger interface {
	WithError(context.Context, error) context.Context
	Warn(context.Context, string)
}

type statIncer interface {
	Inc(ctx context.Context, chatID, userID int64, op stats.OperationType, opts ...stats.Option)
}

type botReplier interface {
	Reply(to *telebot.Message, what interface{}, opts ...interface{}) (*telebot.Message, error)
}
