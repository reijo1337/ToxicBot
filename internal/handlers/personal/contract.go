//go:generate go tool go.uber.org/mock/mockgen -source $GOFILE -destination mocks_test.go -package ${GOPACKAGE}
package personal

import (
	"context"

	"github.com/reijo1337/ToxicBot/internal/features/stats"
)

type messageRepository interface {
	GetEnabledMessages() ([]string, error)
}

type statIncer interface {
	Inc(ctx context.Context, chatID, userID int64, op stats.OperationType, opts ...stats.Option)
}
