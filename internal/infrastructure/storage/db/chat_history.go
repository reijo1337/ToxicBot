package db

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/gob"
	"errors"
	"fmt"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
)

type ChatHistoryStorage struct {
	connGetter connGetter
}

func NewChatHistoryStorage(connGetter connGetter) *ChatHistoryStorage {
	return &ChatHistoryStorage{connGetter: connGetter}
}

func (s *ChatHistoryStorage) Load(ctx context.Context, chatID int64) ([]chathistory.Entry, error) {
	const query = `select data from chat_history where chat_id = ?`

	var blob []byte
	err := s.connGetter.Get(ctx).GetContext(ctx, &blob, query, chatID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to load chat history: %w", err)
	}

	var entries []chathistory.Entry
	if err := gob.NewDecoder(bytes.NewReader(blob)).Decode(&entries); err != nil {
		return nil, fmt.Errorf("failed to decode chat history: %w", err)
	}

	return entries, nil
}

type chatHistoryRow struct {
	ChatID int64  `db:"chat_id"`
	Data   []byte `db:"data"`
}

func (s *ChatHistoryStorage) Save(
	ctx context.Context,
	chatID int64,
	entries []chathistory.Entry,
) error {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(entries); err != nil {
		return fmt.Errorf("failed to encode chat history: %w", err)
	}

	const query = `
insert into chat_history (chat_id, data) values (:chat_id, :data)
on conflict(chat_id) do update set data = excluded.data`

	_, err := s.connGetter.Get(ctx).NamedExecContext(ctx, query, chatHistoryRow{
		ChatID: chatID,
		Data:   buf.Bytes(),
	})
	if err != nil {
		return fmt.Errorf("failed to save chat history: %w", err)
	}

	return nil
}
