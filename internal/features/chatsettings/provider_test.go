package chatsettings

import (
	"context"
	"fmt"
	"testing"

	"github.com/reijo1337/ToxicBot/internal/domain/chat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type settingsRepository struct {
	settings map[int64]chat.ChatSettings
}

func ptr[T any](value T) *T { return &value }

func (r *settingsRepository) Get(_ context.Context, chatID int64) (*chat.ChatSettings, error) {
	s, ok := r.settings[chatID]
	if !ok {
		return nil, nil
	}
	return &s, nil
}

func (r *settingsRepository) Upsert(_ context.Context, chatID int64, s chat.ChatSettings) error {
	r.settings[chatID] = s
	return nil
}

func (r *settingsRepository) Delete(_ context.Context, chatID int64) error {
	delete(r.settings, chatID)
	return nil
}

// spec: SET-019
// spec: SET-020
// spec: SET-022
func TestProvider_PhotoChancePriorities(t *testing.T) {
	t.Parallel()
	for _, global := range []float32{1, 0, 0.4} {
		for _, tc := range []struct {
			name    string
			partial *chat.ChatSettings
			want    float32
		}{
			{"no_settings", nil, global},
			{"other_setting", &chat.ChatSettings{ThresholdCount: ptr(7)}, global},
			{"saved_zero", &chat.ChatSettings{PhotoReactChance: ptr(float32(0))}, 0},
			{"saved_old_default", &chat.ChatSettings{PhotoReactChance: ptr(float32(0.75))}, 0.75},
		} {
			t.Run(fmt.Sprintf("%g/%s", global, tc.name), func(t *testing.T) {
				t.Parallel()
				repo := &settingsRepository{settings: make(map[int64]chat.ChatSettings)}
				if tc.partial != nil {
					repo.settings[42] = *tc.partial
				}
				p := NewProvider(repo, Defaults{PhotoReactChance: global})
				s, err := p.GetForChat(t.Context(), 42)
				require.NoError(t, err)
				assert.InDelta(t, tc.want, s.PhotoReactChance, 0.001)
			})
		}
	}
}

// spec: SET-021
// spec: SET-022
func TestProvider_ResetPhotoChance(t *testing.T) {
	t.Parallel()
	for _, global := range []float32{1, 0, 0.4} {
		t.Run(fmt.Sprint(global), func(t *testing.T) {
			t.Parallel()
			repo := &settingsRepository{settings: map[int64]chat.ChatSettings{
				42: {PhotoReactChance: ptr(float32(0.75))},
			}}
			p := NewProvider(repo, Defaults{PhotoReactChance: global})
			s, err := p.GetForChat(t.Context(), 42)
			require.NoError(t, err)
			assert.InDelta(t, 0.75, s.PhotoReactChance, 0.001)
			require.NoError(t, p.ResetChat(t.Context(), 42))
			s, err = p.GetForChat(t.Context(), 42)
			require.NoError(t, err)
			assert.InDelta(t, global, s.PhotoReactChance, 0.001)
			assert.Empty(t, repo.settings)
		})
	}
}
