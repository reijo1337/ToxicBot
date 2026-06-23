//go:generate go tool go.uber.org/mock/mockgen -source $GOFILE -destination mocks_test.go -package ${GOPACKAGE}
package bulling

import (
	"context"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/reijo1337/ToxicBot/internal/features/message"
	"github.com/reijo1337/ToxicBot/internal/features/stats"
	"gopkg.in/telebot.v3"
)

type messageGenerator interface {
	GetMessageTextWithHistory(
		ctx context.Context,
		history []chathistory.Entry,
		aiChance float32,
		forceAI bool,
	) message.GenerationResult
}

type statIncer interface {
	Inc(ctx context.Context, chatID, userID int64, op stats.OperationType, opts ...stats.Option)
}

type historyBuffer interface {
	Add(chatID int64, e chathistory.Entry)
	AddAll(chatID int64, entries ...chathistory.Entry)
	Get(chatID int64) []chathistory.Entry
}

type botReplier interface {
	Reply(to *telebot.Message, what interface{}, opts ...interface{}) (*telebot.Message, error)
}
