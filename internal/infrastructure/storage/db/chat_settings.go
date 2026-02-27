package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/reijo1337/ToxicBot/internal/domain/chat"
)

type chatSettingsRow struct {
	ChatID             int64    `db:"chat_id"`
	ThresholdCount     *int64   `db:"threshold_count"`
	ThresholdTimeNs    *int64   `db:"threshold_time_ns"`
	CooldownNs         *int64   `db:"cooldown_ns"`
	StickerReactChance *float64 `db:"sticker_react_chance"`
	VoiceReactChance   *float64 `db:"voice_react_chance"`
	AIChance           *float64 `db:"ai_chance"`
}

type ChatSettingsStorage struct {
	connGetter connGetter
}

func NewChatSettingsStorage(connGetter connGetter) *ChatSettingsStorage {
	return &ChatSettingsStorage{connGetter: connGetter}
}

func (s *ChatSettingsStorage) Get(ctx context.Context, chatID int64) (*chat.ChatSettings, error) {
	const query = `
select
	chat_id
	,threshold_count
	,threshold_time_ns
	,cooldown_ns
	,sticker_react_chance
	,voice_react_chance
	,ai_chance
from chat_settings
where chat_id = ?`

	var row chatSettingsRow
	err := s.connGetter.Get(ctx).GetContext(ctx, &row, query, chatID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get chat settings: %w", err)
	}

	return rowToDomain(row), nil
}

func (s *ChatSettingsStorage) Upsert(
	ctx context.Context,
	chatID int64,
	settings chat.ChatSettings,
) error {
	row := domainToRow(chatID, settings)

	const query = `
insert into chat_settings (
	chat_id
	,threshold_count
	,threshold_time_ns
	,cooldown_ns
	,sticker_react_chance
	,voice_react_chance
	,ai_chance
	,updated_at
) values (
	:chat_id
	,:threshold_count
	,:threshold_time_ns
	,:cooldown_ns
	,:sticker_react_chance
	,:voice_react_chance
	,:ai_chance
	,current_timestamp
) on conflict(chat_id) do update set
	threshold_count      = coalesce(:threshold_count,      threshold_count)
	,threshold_time_ns   = coalesce(:threshold_time_ns,    threshold_time_ns)
	,cooldown_ns         = coalesce(:cooldown_ns,          cooldown_ns)
	,sticker_react_chance = coalesce(:sticker_react_chance, sticker_react_chance)
	,voice_react_chance   = coalesce(:voice_react_chance,   voice_react_chance)
	,ai_chance            = coalesce(:ai_chance,            ai_chance)
	,updated_at          = current_timestamp`

	_, err := s.connGetter.Get(ctx).NamedExecContext(ctx, query, row)
	if err != nil {
		return fmt.Errorf("failed to upsert chat settings: %w", err)
	}

	return nil
}

func (s *ChatSettingsStorage) Delete(ctx context.Context, chatID int64) error {
	const query = `delete from chat_settings where chat_id = :chat_id`

	_, err := s.connGetter.Get(ctx).NamedExecContext(ctx, query, map[string]any{"chat_id": chatID})
	if err != nil {
		return fmt.Errorf("failed to delete chat settings: %w", err)
	}

	return nil
}

func rowToDomain(row chatSettingsRow) *chat.ChatSettings {
	out := &chat.ChatSettings{}

	if row.ThresholdCount != nil {
		v := int(*row.ThresholdCount)
		out.ThresholdCount = &v
	}

	if row.ThresholdTimeNs != nil {
		v := time.Duration(*row.ThresholdTimeNs)
		out.ThresholdTime = &v
	}

	if row.CooldownNs != nil {
		v := time.Duration(*row.CooldownNs)
		out.Cooldown = &v
	}

	if row.StickerReactChance != nil {
		v := float32(*row.StickerReactChance)
		out.StickerReactChance = &v
	}

	if row.VoiceReactChance != nil {
		v := float32(*row.VoiceReactChance)
		out.VoiceReactChance = &v
	}

	if row.AIChance != nil {
		v := float32(*row.AIChance)
		out.AIChance = &v
	}

	return out
}

func domainToRow(chatID int64, s chat.ChatSettings) chatSettingsRow {
	row := chatSettingsRow{ChatID: chatID}

	if s.ThresholdCount != nil {
		v := int64(*s.ThresholdCount)
		row.ThresholdCount = &v
	}

	if s.ThresholdTime != nil {
		v := s.ThresholdTime.Nanoseconds()
		row.ThresholdTimeNs = &v
	}

	if s.Cooldown != nil {
		v := s.Cooldown.Nanoseconds()
		row.CooldownNs = &v
	}

	if s.StickerReactChance != nil {
		v := float64(*s.StickerReactChance)
		row.StickerReactChance = &v
	}

	if s.VoiceReactChance != nil {
		v := float64(*s.VoiceReactChance)
		row.VoiceReactChance = &v
	}

	if s.AIChance != nil {
		v := float64(*s.AIChance)
		row.AIChance = &v
	}

	return row
}
