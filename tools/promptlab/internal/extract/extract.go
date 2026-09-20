// Package extract достаёт окна истории из прод-копии SQLite: chat_history хранит
// по одному gob-блобу на чат — последние записи буфера, ровно то, что видел генератор.
package extract

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/gob"
	"fmt"

	// Драйвер sqlite3 регистрируется побочным эффектом импорта, как в cmd/main.go.
	_ "github.com/mattn/go-sqlite3"
	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/reijo1337/ToxicBot/tools/promptlab/internal/cases"
)

type Options struct {
	// Chats — если непустой, берутся только эти chat_id.
	Chats map[int64]struct{}
	// MinHistory — минимум записей перед ответом бота, чтобы окно стало кейсом.
	MinHistory int
}

func Run(ctx context.Context, dbPath string, opts Options) ([]cases.Case, error) {
	dsn := fmt.Sprintf("file:%s?mode=ro", dbPath)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("открыть %s: %w", dbPath, err)
	}
	defer db.Close() //nolint:errcheck // база только на чтение

	rows, err := db.QueryContext(ctx, `select chat_id, data from chat_history order by chat_id`)
	if err != nil {
		return nil, fmt.Errorf("прочитать chat_history: %w", err)
	}
	defer rows.Close() //nolint:errcheck // курсор только на чтение

	var out []cases.Case
	for rows.Next() {
		var (
			chatID int64
			blob   []byte
		)
		if err := rows.Scan(&chatID, &blob); err != nil {
			return nil, fmt.Errorf("прочитать строку: %w", err)
		}
		if len(opts.Chats) > 0 {
			if _, ok := opts.Chats[chatID]; !ok {
				continue
			}
		}

		var entries []chathistory.Entry
		if err := gob.NewDecoder(bytes.NewReader(blob)).Decode(&entries); err != nil {
			return nil, fmt.Errorf("декодировать историю чата %d: %w", chatID, err)
		}

		out = append(out, cases.FromHistory(chatID, entries, opts.MinHistory)...)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("обход chat_history: %w", err)
	}

	return out, nil
}
