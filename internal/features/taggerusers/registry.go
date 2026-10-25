// Package taggerusers хранит кандидатов теггера — известных боту участников чатов.
package taggerusers

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/reijo1337/ToxicBot/internal/domain/chat"
)

type Store interface {
	// LoadAll возвращает каждую пару (chat_id, user_id) не больше одного раза.
	LoadAll(ctx context.Context) ([]chat.TaggerUser, error)
	Upsert(ctx context.Context, user chat.TaggerUser) error
	Delete(ctx context.Context, chatID, userID int64) error
	DeleteChat(ctx context.Context, chatID int64) error
}

type memberKey struct {
	chatID int64
	userID int64
}

// Registry держит список в памяти и синхронно пишет каждое изменение в хранилище до
// обновления памяти. При ошибке записи память остаётся прежней.
type Registry struct {
	store   Store
	chats   map[int64][]int64
	authors map[memberKey]string
	mu      sync.RWMutex
}

// New поднимает весь сохранённый список.
func New(ctx context.Context, store Store) (*Registry, error) {
	users, err := store.LoadAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("can't load tagger users: %w", err)
	}

	r := &Registry{
		store:   store,
		chats:   make(map[int64][]int64, 10),
		authors: make(map[memberKey]string, max(len(users), 2_000)),
	}
	for _, u := range users {
		r.authors[memberKey{chatID: u.ChatID, userID: u.UserID}] = u.Author
		r.chats[u.ChatID] = append(r.chats[u.ChatID], u.UserID)
	}

	return r, nil
}

// Chats возвращает чаты, в которых есть хотя бы один участник.
func (r *Registry) Chats() []int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]int64, 0, len(r.chats))
	for chatID := range r.chats {
		out = append(out, chatID)
	}

	return out
}

// Users возвращает копию списка участников чата.
func (r *Registry) Users(chatID int64) []int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return slices.Clone(r.chats[chatID])
}

// Author возвращает подпись участника и признак того, что он в списке.
func (r *Registry) Author(chatID, userID int64) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	author, ok := r.authors[memberKey{chatID: chatID, userID: userID}]
	return author, ok
}

// Add добавляет участника или обновляет его подпись. Неизменившаяся подпись не пишется.
func (r *Registry) Add(ctx context.Context, user chat.TaggerUser) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := memberKey{chatID: user.ChatID, userID: user.UserID}
	prev, known := r.authors[key]
	if known && prev == user.Author {
		return nil
	}

	if err := r.store.Upsert(ctx, user); err != nil {
		return fmt.Errorf("can't save tagger user: %w", err)
	}

	r.authors[key] = user.Author
	if !known {
		r.chats[user.ChatID] = append(r.chats[user.ChatID], user.UserID)
	}

	return nil
}

// Remove удаляет участника; неизвестный участник хранилище не трогает.
func (r *Registry) Remove(ctx context.Context, chatID, userID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := memberKey{chatID: chatID, userID: userID}
	if _, ok := r.authors[key]; !ok {
		return nil
	}

	if err := r.store.Delete(ctx, chatID, userID); err != nil {
		return fmt.Errorf("can't delete tagger user: %w", err)
	}

	delete(r.authors, key)

	users := r.chats[chatID]
	if i := slices.Index(users, userID); i >= 0 {
		users[i] = users[len(users)-1]
		users = users[:len(users)-1]
	}

	if len(users) == 0 {
		delete(r.chats, chatID)
		return nil
	}
	r.chats[chatID] = users

	return nil
}

// RemoveChat удаляет всех участников чата.
func (r *Registry) RemoveChat(ctx context.Context, chatID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.store.DeleteChat(ctx, chatID); err != nil {
		return fmt.Errorf("can't delete tagger chat: %w", err)
	}

	for _, userID := range r.chats[chatID] {
		delete(r.authors, memberKey{chatID: chatID, userID: userID})
	}
	delete(r.chats, chatID)

	return nil
}
