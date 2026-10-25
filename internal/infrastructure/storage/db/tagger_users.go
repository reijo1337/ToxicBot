package db

import (
	"context"
	"fmt"

	"github.com/reijo1337/ToxicBot/internal/domain/chat"
)

type taggerUserRow struct {
	Author string `db:"author"`
	ChatID int64  `db:"chat_id"`
	UserID int64  `db:"user_id"`
}

type TaggerUsersStorage struct {
	connGetter connGetter
}

func NewTaggerUsersStorage(connGetter connGetter) *TaggerUsersStorage {
	return &TaggerUsersStorage{connGetter: connGetter}
}

func (s *TaggerUsersStorage) LoadAll(ctx context.Context) ([]chat.TaggerUser, error) {
	const query = `select chat_id, user_id, author from tagger_users order by chat_id, user_id`

	var rows []taggerUserRow
	if err := s.connGetter.Get(ctx).SelectContext(ctx, &rows, query); err != nil {
		return nil, fmt.Errorf("load tagger users: %w", err)
	}

	out := make([]chat.TaggerUser, 0, len(rows))
	for _, r := range rows {
		out = append(out, chat.TaggerUser{Author: r.Author, ChatID: r.ChatID, UserID: r.UserID})
	}

	return out, nil
}

func (s *TaggerUsersStorage) Upsert(ctx context.Context, user chat.TaggerUser) error {
	const query = `
insert into tagger_users (chat_id, user_id, author) values (:chat_id, :user_id, :author)
on conflict(chat_id, user_id) do update set author = excluded.author`

	_, err := s.connGetter.Get(ctx).NamedExecContext(ctx, query, taggerUserRow{
		Author: user.Author,
		ChatID: user.ChatID,
		UserID: user.UserID,
	})
	if err != nil {
		return fmt.Errorf("upsert tagger user: %w", err)
	}

	return nil
}

func (s *TaggerUsersStorage) Delete(ctx context.Context, chatID, userID int64) error {
	const query = `delete from tagger_users where chat_id = :chat_id and user_id = :user_id`

	_, err := s.connGetter.Get(ctx).NamedExecContext(ctx, query, map[string]any{
		"chat_id": chatID,
		"user_id": userID,
	})
	if err != nil {
		return fmt.Errorf("delete tagger user: %w", err)
	}

	return nil
}

func (s *TaggerUsersStorage) DeleteChat(ctx context.Context, chatID int64) error {
	const query = `delete from tagger_users where chat_id = :chat_id`

	_, err := s.connGetter.Get(ctx).NamedExecContext(ctx, query, map[string]any{"chat_id": chatID})
	if err != nil {
		return fmt.Errorf("delete tagger chat: %w", err)
	}

	return nil
}
