package on_sticker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/reijo1337/ToxicBot/internal/features/chatsettings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gopkg.in/telebot.v3"
)

type attributionContext struct {
	telebot.Context

	sender    *telebot.User
	msg       *telebot.Message
	notifyErr error
}

func (c *attributionContext) Chat() *telebot.Chat             { return &telebot.Chat{ID: 7} }
func (c *attributionContext) Sender() *telebot.User           { return c.sender }
func (c *attributionContext) Message() *telebot.Message       { return c.msg }
func (c *attributionContext) Notify(telebot.ChatAction) error { return c.notifyErr }
func (*attributionContext) Set(string, interface{})           {}
func (*attributionContext) Get(string) interface{}            { return nil }

type attributionSettings struct {
	settings *chatsettings.Settings
	err      error
}

func (s attributionSettings) GetForChat(context.Context, int64) (*chatsettings.Settings, error) {
	return s.settings, s.err
}

type attributionEnv struct {
	handler  *StickerReactions
	random   *Mockrandomizer
	replier  *MockbotReplier
	history  *chathistory.Buffer
	contexts []*attributionContext
}

func newAttributionEnv(t *testing.T) *attributionEnv {
	t.Helper()
	ctrl := gomock.NewController(t)
	env := &attributionEnv{
		random:  NewMockrandomizer(ctrl),
		replier: NewMockbotReplier(ctrl),
		history: chathistory.NewBuffer(100),
	}
	stat := NewMockstatIncer(ctrl)
	stat.EXPECT().Inc(gomock.Any(), int64(7), gomock.Any(), gomock.Any()).AnyTimes()
	env.handler = &StickerReactions{
		ctx:       context.Background(),
		r:         env.random,
		statIncer: stat,
		settingsProvider: attributionSettings{
			settings: &chatsettings.Settings{StickerReactChance: 1},
		},
		history:  env.history,
		replier:  env.replier,
		stickers: []string{"sticker"},
	}
	for i, senderID := range []int64{101, 202} {
		env.contexts = append(env.contexts, &attributionContext{
			sender: &telebot.User{ID: senderID, FirstName: "Alex"},
			msg: &telebot.Message{
				ID:       i + 1,
				Unixtime: time.Now().Unix(),
				ReplyTo:  &telebot.Message{ID: 50},
			},
		})
	}
	return env
}

func (env *attributionEnv) expectReaction(t *testing.T) {
	t.Helper()
	env.random.EXPECT().Float32().Return(float32(0)).Times(2)
	env.random.EXPECT().Intn(1).Return(0).Times(2)
}

func (env *attributionEnv) handleSenders(t *testing.T, wantErr error) {
	t.Helper()
	for _, ctx := range env.contexts {
		require.ErrorIs(t, env.handler.Handle(ctx), wantErr)
	}
}

func (env *attributionEnv) assertUserAuthorship(t *testing.T, wantEntries int) {
	t.Helper()
	entries := env.history.Get(7)
	require.Len(t, entries, wantEntries)
	var users []chathistory.Entry
	for _, entry := range entries {
		if entry.FromBot {
			assert.Zero(t, entry.SenderID)
			continue
		}
		users = append(users, entry)
	}
	require.Len(t, users, 2)
	assert.Equal(t, int64(101), users[0].SenderID)
	assert.Equal(t, int64(202), users[1].SenderID)
	assert.Equal(t, users[0].Author, users[1].Author)
	assert.Equal(t, 50, users[0].ReplyToID)
}

// spec: STK-008
func TestHandle_SameNameSendersOnSkip(t *testing.T) {
	t.Parallel()
	env := newAttributionEnv(t)
	env.handler.settingsProvider = attributionSettings{settings: &chatsettings.Settings{}}
	env.random.EXPECT().Float32().Return(float32(1)).Times(2)
	env.handleSenders(t, nil)
	env.assertUserAuthorship(t, 2)
}

// spec: STK-008
func TestHandle_SameNameSendersOnSuccess(t *testing.T) {
	t.Parallel()
	env := newAttributionEnv(t)
	env.expectReaction(t)
	for _, ctx := range env.contexts {
		env.replier.EXPECT().
			Reply(ctx.msg, gomock.Any()).
			Return(&telebot.Message{ID: 100 + ctx.msg.ID}, nil)
	}
	env.handleSenders(t, nil)
	env.assertUserAuthorship(t, 4)
}

// spec: STK-008
func TestHandle_SameNameSendersOnReplyError(t *testing.T) {
	t.Parallel()
	env := newAttributionEnv(t)
	env.expectReaction(t)
	sendErr := errors.New("send")
	env.replier.EXPECT().Reply(gomock.Any(), gomock.Any()).Return(nil, sendErr).Times(2)
	env.handleSenders(t, sendErr)
	env.assertUserAuthorship(t, 2)
}

// spec: STK-008
func TestHandle_SameNameSendersOnSettingsError(t *testing.T) {
	t.Parallel()
	env := newAttributionEnv(t)
	settingsErr := errors.New("settings")
	env.handler.settingsProvider = attributionSettings{err: settingsErr}
	env.handleSenders(t, settingsErr)
	env.assertUserAuthorship(t, 2)
}

// spec: HIST-022
func TestHandle_UnconfirmedSenderNeverGetsPersonalAuthorship(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		sender     *telebot.User
		senderChat *telebot.Chat
	}{
		{name: "channel", sender: &telebot.User{ID: 101, FirstName: "Alex"}, senderChat: &telebot.Chat{ID: -10}},
		{name: "bot", sender: &telebot.User{ID: 101, IsBot: true}},
		{name: "missing"},
		{name: "zero_id", sender: &telebot.User{FirstName: "Alex"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			history := chathistory.NewBuffer(10)
			random := NewMockrandomizer(ctrl)
			random.EXPECT().Float32().Return(float32(1))
			h := &StickerReactions{
				ctx: context.Background(),
				r:   random,
				settingsProvider: attributionSettings{
					settings: &chatsettings.Settings{},
				},
				history: history,
			}
			require.NoError(t, h.Handle(&attributionContext{
				sender: tc.sender,
				msg:    &telebot.Message{ID: 1, SenderChat: tc.senderChat},
			}))
			entries := history.Get(7)
			require.Len(t, entries, 1)
			assert.Zero(t, entries[0].SenderID)
		})
	}
}
