package chatsettings

import (
	"context"
	"sync"
	"time"

	"github.com/reijo1337/ToxicBot/internal/domain/chat"
)

type Defaults struct {
	ThresholdCount     int
	ThresholdTime      time.Duration
	Cooldown           time.Duration
	StickerReactChance float32
	VoiceReactChance   float32
	AIChance           float32
}

type Settings struct {
	ThresholdCount     int
	ThresholdTime      time.Duration
	Cooldown           time.Duration
	StickerReactChance float32
	VoiceReactChance   float32
	AIChance           float32
}

type repository interface {
	Get(ctx context.Context, chatID int64) (*chat.ChatSettings, error)
	Upsert(ctx context.Context, chatID int64, s chat.ChatSettings) error
	Delete(ctx context.Context, chatID int64) error
}

type cachedEntry struct {
	settings  *chat.ChatSettings
	expiresAt time.Time
}

type Provider struct {
	repo     repository
	defaults Defaults
	cache    sync.Map
	cacheTTL time.Duration
}

func NewProvider(repo repository, defaults Defaults) *Provider {
	return &Provider{
		repo:     repo,
		defaults: defaults,
		cacheTTL: time.Minute,
	}
}

func (p *Provider) GetForChat(ctx context.Context, chatID int64) (*Settings, error) {
	chatSettings, err := p.getChatSettings(ctx, chatID)
	if err != nil {
		return nil, err
	}

	return p.merge(chatSettings), nil
}

func (p *Provider) UpsertForChat(
	ctx context.Context,
	chatID int64,
	partial chat.ChatSettings,
) error {
	if err := p.repo.Upsert(ctx, chatID, partial); err != nil {
		return err
	}

	p.cache.Delete(chatID)

	return nil
}

func (p *Provider) ResetChat(ctx context.Context, chatID int64) error {
	if err := p.repo.Delete(ctx, chatID); err != nil {
		return err
	}

	p.cache.Delete(chatID)

	return nil
}

func (p *Provider) getChatSettings(ctx context.Context, chatID int64) (*chat.ChatSettings, error) {
	if v, ok := p.cache.Load(chatID); ok {
		entry := v.(cachedEntry)
		if time.Now().Before(entry.expiresAt) {
			return entry.settings, nil
		}

		p.cache.Delete(chatID)
	}

	settings, err := p.repo.Get(ctx, chatID)
	if err != nil {
		return nil, err
	}

	p.cache.Store(chatID, cachedEntry{
		settings:  settings,
		expiresAt: time.Now().Add(p.cacheTTL),
	})

	return settings, nil
}

func (p *Provider) merge(chatSettings *chat.ChatSettings) *Settings {
	out := &Settings{
		ThresholdCount:     p.defaults.ThresholdCount,
		ThresholdTime:      p.defaults.ThresholdTime,
		Cooldown:           p.defaults.Cooldown,
		StickerReactChance: p.defaults.StickerReactChance,
		VoiceReactChance:   p.defaults.VoiceReactChance,
		AIChance:           p.defaults.AIChance,
	}

	if chatSettings == nil {
		return out
	}

	if chatSettings.ThresholdCount != nil {
		out.ThresholdCount = *chatSettings.ThresholdCount
	}

	if chatSettings.ThresholdTime != nil {
		out.ThresholdTime = *chatSettings.ThresholdTime
	}

	if chatSettings.Cooldown != nil {
		out.Cooldown = *chatSettings.Cooldown
	}

	if chatSettings.StickerReactChance != nil {
		out.StickerReactChance = *chatSettings.StickerReactChance
	}

	if chatSettings.VoiceReactChance != nil {
		out.VoiceReactChance = *chatSettings.VoiceReactChance
	}

	if chatSettings.AIChance != nil {
		out.AIChance = *chatSettings.AIChance
	}

	return out
}
