package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/reijo1337/ToxicBot/internal/domain/chat"
	"github.com/reijo1337/ToxicBot/internal/features/chatsettings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/telebot.v3"
)

const (
	testChatID = int64(-100)
	testUserID = int64(42)
)

type providerCall struct {
	method  string
	chatID  int64
	partial chat.ChatSettings
}

type providerStub struct {
	settings chatsettings.Settings
	calls    []providerCall
}

func (p *providerStub) GetForChat(_ context.Context, chatID int64) (*chatsettings.Settings, error) {
	p.calls = append(p.calls, providerCall{method: "get", chatID: chatID})
	return &p.settings, nil
}

func (p *providerStub) UpsertForChat(
	_ context.Context,
	chatID int64,
	partial chat.ChatSettings,
) error {
	p.calls = append(p.calls, providerCall{method: "upsert", chatID: chatID, partial: partial})
	return nil
}

func (p *providerStub) ResetChat(_ context.Context, chatID int64) error {
	p.calls = append(p.calls, providerCall{method: "reset", chatID: chatID})
	return nil
}

type apiRequest struct {
	method string
	params map[string]string
}

type telegramAPI struct {
	mu            sync.Mutex
	requests      []apiRequest
	roleResponses []string
	sendResponse  string
}

func (a *telegramAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	var params map[string]string
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	method := strings.TrimPrefix(r.URL.Path, "/bottest/")
	a.requests = append(a.requests, apiRequest{method: method, params: params})
	w.Header().Set("Content-Type", "application/json")

	response := `{"ok":true,"result":{"message_id":2,"chat":{"id":-100,"type":"supergroup"}}}`
	switch method {
	case "getChatMember":
		response = `{"ok":true,"result":{"status":"administrator","user":{"id":42}}}`
		if len(a.roleResponses) > 0 {
			response = a.roleResponses[0]
			a.roleResponses = a.roleResponses[1:]
		}
	case "sendMessage":
		if a.sendResponse != "" {
			response = a.sendResponse
		}
	default:
		http.Error(w, "unexpected Telegram method", http.StatusNotFound)
		return
	}
	_, _ = w.Write([]byte(response))
}

func (a *telegramAPI) calls() []apiRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]apiRequest(nil), a.requests...)
}

func newTestHandler(t *testing.T, api *telegramAPI) (*Handler, *telebot.Bot, *providerStub) {
	t.Helper()
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)
	bot, err := telebot.NewBot(telebot.Settings{
		Token: "test", URL: server.URL, Offline: true, Client: server.Client(),
	})
	require.NoError(t, err)
	defaults := chatsettings.Defaults{
		ThresholdCount: 5, ThresholdTime: time.Minute, Cooldown: time.Hour,
		StickerReactChance: 0.4, VoiceReactChance: 0.8, AIChance: 0.75, PhotoReactChance: 1,
		TaggerEnabled: true,
	}
	provider := &providerStub{settings: chatsettings.Settings(defaults)}
	return New(provider, defaults), bot, provider
}

func command(text string, chatType telebot.ChatType) *telebot.Message {
	return &telebot.Message{
		ID: 1, Text: text,
		Chat: &telebot.Chat{ID: testChatID, Type: chatType}, Sender: &telebot.User{ID: testUserID},
	}
}

func runCommand(h *Handler, bot *telebot.Bot, msg *telebot.Message) error {
	return h.Handle(bot.NewContext(telebot.Update{Message: msg}))
}

func roleResponse(role telebot.MemberStatus) string {
	return fmt.Sprintf(`{"ok":true,"result":{"status":%q,"user":{"id":42}}}`, role)
}

func assertRoleRequest(t *testing.T, request apiRequest, chatID int64) {
	t.Helper()
	assert.Equal(t, "getChatMember", request.method)
	assert.Equal(t, strconv.FormatInt(chatID, 10), request.params["chat_id"])
	assert.Equal(t, "42", request.params["user_id"])
}

// spec: SET-005
// spec: SET-006
// spec: SET-007
// spec: SET-009
// spec: SET-013
func TestHandle_AdminCommands(t *testing.T) {
	t.Parallel()
	forms := []struct {
		text   string
		method string
		reply  string
	}{
		{"/settings", "get", "threshold_count: 5 (по умолчанию: 5)"},
		{"/settings photo_chance 0.75", "upsert", "photo_chance = 0.75"},
		{"/settings reset", "reset", "сброшено"},
		{"/settings help", "", "Использование:"},
		{"/settings a b c", "", "Использование:"},
		{"/settings unknown 1", "", "Такой настройки не существует, балбес.\n\nИспользование:"},
		{"/settings@toxic_test_bot", "get", "Настройки чата:"},
	}
	for _, role := range []telebot.MemberStatus{telebot.Creator, telebot.Administrator} {
		for _, chatType := range []telebot.ChatType{telebot.ChatGroup, telebot.ChatSuperGroup} {
			for _, form := range forms {
				t.Run(string(role)+"/"+string(chatType)+"/"+form.text, func(t *testing.T) {
					t.Parallel()
					api := &telegramAPI{roleResponses: []string{roleResponse(role)}}
					h, bot, provider := newTestHandler(t, api)
					require.NoError(t, runCommand(h, bot, command(form.text, chatType)))
					calls := api.calls()
					require.Len(t, calls, 2)
					assertRoleRequest(t, calls[0], testChatID)
					assert.Equal(t, "sendMessage", calls[1].method)
					assert.Equal(t, "-100", calls[1].params["chat_id"])
					assert.Contains(t, calls[1].params["text"], form.reply)
					if form.method == "" {
						assert.Empty(t, provider.calls)
						return
					}
					require.Len(t, provider.calls, 1)
					assert.Equal(t, form.method, provider.calls[0].method)
					assert.Equal(t, testChatID, provider.calls[0].chatID)
					if form.method == "upsert" {
						require.NotNil(t, provider.calls[0].partial.PhotoReactChance)
						assert.InDelta(t, 0.75, *provider.calls[0].partial.PhotoReactChance, 0.001)
					}
				})
			}
		}
	}
}

// spec: SET-005
func TestHandle_ShowOverrides(t *testing.T) {
	t.Parallel()
	api := &telegramAPI{}
	h, bot, provider := newTestHandler(t, api)
	provider.settings = chatsettings.Settings{
		ThresholdCount: 7, ThresholdTime: 2 * time.Minute, Cooldown: 30 * time.Minute,
		StickerReactChance: 0.2, VoiceReactChance: 0.3, AIChance: 0.4, PhotoReactChance: 0.75,
	}
	require.NoError(t, runCommand(h, bot, command("/settings", telebot.ChatSuperGroup)))
	calls := api.calls()
	require.Len(t, calls, 2)
	for _, line := range []string{
		"threshold_count: 7 (по умолчанию: 5)",
		"threshold_time: 2m0s (по умолчанию: 1m0s)",
		"cooldown: 30m0s (по умолчанию: 1h0m0s)",
		"sticker_chance: 0.20 (по умолчанию: 0.40)",
		"voice_chance: 0.30 (по умолчанию: 0.80)",
		"ai_chance: 0.40 (по умолчанию: 0.75)",
		"photo_chance: 0.75 (по умолчанию: 1.00)",
	} {
		assert.Contains(t, calls[1].params["text"], line)
	}
}

// spec: SET-008
// spec: SET-013
// spec: SET-024
func TestHandle_AdminInvalidValues(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ text, example string }{
		{"/settings threshold_count 0", "Пример: 5"},
		{"/settings threshold_count invalid", "Пример: 5"},
		{"/settings threshold_time 0s", "1m, 30s, 2h"},
		{"/settings threshold_time invalid", "1m, 30s, 2h"},
		{"/settings cooldown -1h", "1h, 30m, 15s"},
		{"/settings cooldown invalid", "1h, 30m, 15s"},
		{"/settings sticker_chance 2", "Пример: 0.4"},
		{"/settings voice_chance -0.1", "Пример: 0.8"},
		{"/settings ai_chance invalid", "Пример: 0.75"},
		{"/settings photo_chance 1.1", "Пример: 0.5"},
		{"/settings tagger yes", "Пример: /settings tagger off"},
		{"/settings tagger ON", "Пример: /settings tagger off"},
	} {
		t.Run(tc.text, func(t *testing.T) {
			t.Parallel()
			api := &telegramAPI{}
			h, bot, provider := newTestHandler(t, api)
			require.NoError(t, runCommand(h, bot, command(tc.text, telebot.ChatSuperGroup)))
			assert.Empty(t, provider.calls)
			calls := api.calls()
			require.Len(t, calls, 2)
			assertRoleRequest(t, calls[0], testChatID)
			assert.Contains(t, calls[1].params["text"], tc.example)
		})
	}
}

// spec: SET-014
func TestHandle_NonAdminsDenied(t *testing.T) {
	t.Parallel()
	for _, role := range []telebot.MemberStatus{telebot.Member, telebot.Restricted, telebot.Left, telebot.Kicked} {
		for _, text := range []string{
			"/settings", "/settings reset", "/settings photo_chance 0", "/settings help",
			"/settings unknown 1", "/settings photo_chance invalid", "/settings a b c",
			"/settings@toxic_test_bot", "/settings@toxic_test_bot reset",
			"/settings@toxic_test_bot photo_chance 0",
		} {
			t.Run(string(role)+"/"+text, func(t *testing.T) {
				t.Parallel()
				api := &telegramAPI{roleResponses: []string{roleResponse(role)}}
				h, bot, provider := newTestHandler(t, api)
				require.NoError(t, runCommand(h, bot, command(text, telebot.ChatSuperGroup)))
				assert.Empty(t, provider.calls)
				calls := api.calls()
				require.Len(t, calls, 2)
				assertRoleRequest(t, calls[0], testChatID)
				assert.Equal(
					t,
					"Настройки чата доступны только администраторам.",
					calls[1].params["text"],
				)
			})
		}
	}
}

// spec: SET-015
func TestHandle_VerificationFailure(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, response string }{
		{"api_error", `{"ok":false,"error_code":400,"description":"Bad Request: member lookup failed"}`},
		{"malformed", `{"ok":true,"result":`},
		{"wrong_result_type", `{"ok":true,"result":[]}`},
		{"null", `{"ok":true,"result":null}`},
		{"missing_result", `{"ok":true}`},
		{"missing_role", `{"ok":true,"result":{"user":{"id":42}}}`},
		{"unknown_role", roleResponse("future_role")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			api := &telegramAPI{roleResponses: []string{tc.response}}
			h, bot, provider := newTestHandler(t, api)
			err := runCommand(h, bot, command("/settings reset", telebot.ChatSuperGroup))
			require.ErrorContains(t, err, "check chat administrator")
			assert.Empty(t, provider.calls)
			calls := api.calls()
			require.Len(t, calls, 2)
			assert.Equal(
				t,
				"Не удалось проверить права. Попробуй ещё раз позже.",
				calls[1].params["text"],
			)
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// spec: SET-015
func TestHandle_NetworkFailure(t *testing.T) {
	t.Parallel()
	api := &telegramAPI{}
	h, _, provider := newTestHandler(t, api)
	transportErr := errors.New("connection interrupted")
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)
	client := server.Client()
	transport := client.Transport
	client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/getChatMember") {
			return nil, transportErr
		}
		return transport.RoundTrip(r)
	})
	bot, err := telebot.NewBot(
		telebot.Settings{Token: "test", URL: server.URL, Offline: true, Client: client},
	)
	require.NoError(t, err)
	err = runCommand(h, bot, command("/settings", telebot.ChatGroup))
	require.ErrorIs(t, err, transportErr)
	assert.Empty(t, provider.calls)
	calls := api.calls()
	require.Len(t, calls, 1)
	assert.Equal(t, "Не удалось проверить права. Попробуй ещё раз позже.", calls[0].params["text"])
}

// spec: SET-015
func TestHandle_DenialReplyFailure(t *testing.T) {
	t.Parallel()
	for _, roleReply := range []string{
		roleResponse(telebot.Member),
		`{"ok":false,"error_code":400,"description":"Bad Request: lookup unavailable"}`,
	} {
		t.Run(roleReply, func(t *testing.T) {
			t.Parallel()
			api := &telegramAPI{
				roleResponses: []string{roleReply},
				sendResponse:  `{"ok":false,"error_code":403,"description":"Forbidden: reply rejected"}`,
			}
			h, bot, provider := newTestHandler(t, api)
			err := runCommand(h, bot, command("/settings reset", telebot.ChatSuperGroup))
			require.ErrorContains(t, err, "reply rejected")
			if strings.Contains(roleReply, "lookup unavailable") {
				assert.ErrorContains(t, err, "lookup unavailable")
			}
			assert.Empty(t, provider.calls)
		})
	}
}

// spec: SET-016
func TestHandle_RechecksRoleAndChat(t *testing.T) {
	t.Parallel()
	api := &telegramAPI{roleResponses: []string{
		roleResponse(
			telebot.Administrator,
		),
		roleResponse(telebot.Member),
		roleResponse(telebot.Member),
	}}
	h, bot, provider := newTestHandler(t, api)
	msg := command("/settings", telebot.ChatSuperGroup)
	require.NoError(t, runCommand(h, bot, msg))
	require.NoError(t, runCommand(h, bot, msg))
	msg.Chat = &telebot.Chat{ID: -200, Type: telebot.ChatGroup}
	require.NoError(t, runCommand(h, bot, msg))
	require.Len(t, provider.calls, 1)
	assert.Equal(t, testChatID, provider.calls[0].chatID)
	calls := api.calls()
	require.Len(t, calls, 6)
	assertRoleRequest(t, calls[0], testChatID)
	assertRoleRequest(t, calls[2], testChatID)
	assertRoleRequest(t, calls[4], -200)
	assert.Equal(t, "Настройки чата доступны только администраторам.", calls[3].params["text"])
	assert.Equal(t, "Настройки чата доступны только администраторам.", calls[5].params["text"])
}

// spec: SET-017
func TestHandle_AnonymousAdmin(t *testing.T) {
	t.Parallel()
	for _, sender := range []*telebot.User{nil, {ID: 1087968824, Username: "GroupAnonymousBot"}} {
		for _, tc := range []struct{ text, method string }{
			{"/settings", "get"},
			{"/settings photo_chance 0", "upsert"},
			{"/settings reset", "reset"},
			{"/settings help", ""},
		} {
			t.Run(fmt.Sprintf("%v/%s", sender, tc.text), func(t *testing.T) {
				t.Parallel()
				api := &telegramAPI{}
				h, bot, provider := newTestHandler(t, api)
				msg := command(tc.text, telebot.ChatSuperGroup)
				msg.Sender = sender
				msg.SenderChat = &telebot.Chat{ID: testChatID, Type: telebot.ChatSuperGroup}
				require.NoError(t, runCommand(h, bot, msg))
				calls := api.calls()
				require.Len(t, calls, 1)
				assert.Equal(t, "sendMessage", calls[0].method)
				if tc.method == "" {
					assert.Empty(t, provider.calls)
					assert.Contains(t, calls[0].params["text"], "Использование:")
					return
				}
				require.Len(t, provider.calls, 1)
				assert.Equal(t, tc.method, provider.calls[0].method)
				assert.Equal(t, testChatID, provider.calls[0].chatID)
			})
		}
	}
}

// spec: SET-018
func TestHandle_OtherSenderChatDenied(t *testing.T) {
	t.Parallel()
	for _, sender := range []*telebot.User{nil, {ID: testUserID}} {
		for _, chatType := range []telebot.ChatType{telebot.ChatGroup, telebot.ChatSuperGroup} {
			for _, senderChatID := range []int64{testChatID, -200} {
				if chatType == telebot.ChatSuperGroup && senderChatID == testChatID {
					continue
				}
				for _, text := range []string{"/settings", "/settings reset", "/settings photo_chance 0", "/settings help"} {
					t.Run(
						fmt.Sprintf("%v/%s/%d/%s", sender, chatType, senderChatID, text),
						func(t *testing.T) {
							t.Parallel()
							api := &telegramAPI{}
							h, bot, provider := newTestHandler(t, api)
							msg := command(text, chatType)
							msg.Sender = sender
							msg.SenderChat = &telebot.Chat{
								ID:   senderChatID,
								Type: telebot.ChatChannel,
							}
							require.NoError(t, runCommand(h, bot, msg))
							assert.Empty(t, provider.calls)
							calls := api.calls()
							require.Len(t, calls, 1)
							assert.Equal(
								t,
								"Вызови команду как администратор от своего аккаунта или от имени этой группы.",
								calls[0].params["text"],
							)
						},
					)
				}
			}
		}
	}
}

// spec: SET-010
func TestHandle_PrivateAndUnsupportedChats(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		chatType telebot.ChatType
		reply    string
	}{
		{telebot.ChatPrivate, "Иди в группу, тут тебе не личный кабинет психотерапевта."},
		{telebot.ChatChannel, "Команда доступна только в группе."},
		{telebot.ChatChannelPrivate, "Команда доступна только в группе."},
		{"", "Команда доступна только в группе."},
	} {
		t.Run(string(tc.chatType), func(t *testing.T) {
			t.Parallel()
			api := &telegramAPI{}
			h, bot, provider := newTestHandler(t, api)
			require.NoError(t, runCommand(h, bot, command("/settings", tc.chatType)))
			assert.Empty(t, provider.calls)
			calls := api.calls()
			require.Len(t, calls, 1)
			assert.Equal(t, tc.reply, calls[0].params["text"])
		})
	}
}

func TestHandle_IncompleteContext(t *testing.T) {
	t.Parallel()
	for _, msg := range []*telebot.Message{
		nil, {}, {Chat: &telebot.Chat{ID: testChatID, Type: telebot.ChatSuperGroup}},
	} {
		t.Run(fmt.Sprintf("%+v", msg), func(t *testing.T) {
			t.Parallel()
			api := &telegramAPI{}
			h, bot, provider := newTestHandler(t, api)
			require.NoError(t, runCommand(h, bot, msg))
			assert.Empty(t, provider.calls)
			assert.Empty(t, api.calls())
		})
	}
}

// spec: SET-019
func TestHandle_PhotoChanceDefaultDisplay(t *testing.T) {
	t.Parallel()
	api := &telegramAPI{}
	h, bot, _ := newTestHandler(t, api)
	require.NoError(t, runCommand(h, bot, command("/settings", telebot.ChatSuperGroup)))
	calls := api.calls()
	require.Len(t, calls, 2)
	assert.Contains(t, calls[1].params["text"], "photo_chance: 1.00 (по умолчанию: 1.00)")
}

// spec: SET-023
func TestHandle_TaggerToggle(t *testing.T) {
	t.Parallel()
	for value, want := range map[string]bool{"off": false, "on": true} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			api := &telegramAPI{}
			h, bot, provider := newTestHandler(t, api)
			require.NoError(
				t,
				runCommand(h, bot, command("/settings tagger "+value, telebot.ChatSuperGroup)),
			)
			require.Len(t, provider.calls, 1)
			assert.Equal(t, "upsert", provider.calls[0].method)
			require.NotNil(t, provider.calls[0].partial.TaggerEnabled)
			assert.Equal(t, want, *provider.calls[0].partial.TaggerEnabled)
			calls := api.calls()
			require.Len(t, calls, 2)
			assert.Contains(t, calls[1].params["text"], "tagger = "+value)
		})
	}
}

// spec: SET-025
func TestHandle_TaggerDisplay(t *testing.T) {
	t.Parallel()
	for enabled, line := range map[bool]string{
		true:  "tagger: on (по умолчанию: on)",
		false: "tagger: off (по умолчанию: on)",
	} {
		t.Run(line, func(t *testing.T) {
			t.Parallel()
			api := &telegramAPI{}
			h, bot, provider := newTestHandler(t, api)
			provider.settings.TaggerEnabled = enabled
			require.NoError(t, runCommand(h, bot, command("/settings", telebot.ChatSuperGroup)))
			calls := api.calls()
			require.Len(t, calls, 2)
			assert.Contains(t, calls[1].params["text"], line)
		})
	}

	api := &telegramAPI{}
	h, bot, _ := newTestHandler(t, api)
	require.NoError(t, runCommand(h, bot, command("/settings help", telebot.ChatSuperGroup)))
	assert.Contains(t, api.calls()[1].params["text"], "tagger (on / off)")
}

// spec: SET-026
func TestHandle_TaggerDisplayGlobalDefault(t *testing.T) {
	t.Parallel()
	api := &telegramAPI{}
	h, bot, provider := newTestHandler(t, api)
	h.defaults.TaggerEnabled = false
	provider.settings.TaggerEnabled = false

	require.NoError(t, runCommand(h, bot, command("/settings", telebot.ChatSuperGroup)))

	calls := api.calls()
	require.Len(t, calls, 2)
	assert.Contains(t, calls[1].params["text"], "tagger: off (по умолчанию: off)")
}
