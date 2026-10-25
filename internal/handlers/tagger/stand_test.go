package tagger

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/reijo1337/ToxicBot/internal/domain/chat"
	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/reijo1337/ToxicBot/internal/features/chatsettings"
	"github.com/reijo1337/ToxicBot/internal/features/message"
	"github.com/reijo1337/ToxicBot/internal/features/stats"
	"github.com/reijo1337/ToxicBot/internal/features/taggerusers"
	"github.com/stretchr/testify/require"
	"gopkg.in/telebot.v3"
)

const (
	botID     = int64(999)
	groupA    = int64(-100)
	groupB    = int64(-200)
	tagFrom   = time.Hour
	tagTo     = 2 * time.Hour
	memberRaw = `{"status":"member","user":{"id":1}}`
	leftRaw   = `{"status":"left","user":{"id":1}}`
	sendOK    = `{"ok":true,"result":{"message_id":2,"chat":{"id":-100,"type":"supergroup"}}}`
)

// fakeTelegram — тестовый Bot API: getChatMember отвечает по user_id, sendMessage
// запоминает отправленное.
type fakeTelegram struct {
	members map[string]string // user_id → тело ответа целиком
	holds   map[string]*memberHold
	sendErr bool
	sent    []sentMessage
	checks  []string
	mu      sync.Mutex
}

type sentMessage struct {
	chatID string
	text   string
}

// memberHold задерживает один ответ getChatMember.
type memberHold struct {
	started chan struct{}
	release chan struct{}
}

func (a *fakeTelegram) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var params map[string]string
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	switch strings.TrimPrefix(r.URL.Path, "/bottest/") {
	case "getChatMember":
		resp, hold := a.memberAnswer(params["user_id"])
		if hold != nil {
			close(hold.started)
			<-hold.release
		}
		_, _ = w.Write([]byte(resp))
	case "sendMessage":
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.sendErr {
			_, _ = w.Write(
				[]byte(`{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`),
			)
			return
		}
		a.sent = append(a.sent, sentMessage{chatID: params["chat_id"], text: params["text"]})
		_, _ = w.Write([]byte(sendOK))
	default:
		http.Error(w, "unexpected Telegram method", http.StatusNotFound)
	}
}

// memberAnswer фиксирует ответ в момент запроса, даже если отдан он будет позже.
func (a *fakeTelegram) memberAnswer(userID string) (string, *memberHold) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.checks = append(a.checks, userID)
	resp, ok := a.members[userID]
	if !ok {
		resp = `{"ok":true,"result":` + memberRaw + `}`
	}

	hold := a.holds[userID]
	delete(a.holds, userID)

	return resp, hold
}

// holdMember задерживает следующий ответ getChatMember для userID: started закрывается,
// когда запрос пришёл, а ответ уходит после release.
func (a *fakeTelegram) holdMember(userID int64) (started <-chan struct{}, release func()) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.holds == nil {
		a.holds = make(map[string]*memberHold)
	}
	hold := &memberHold{started: make(chan struct{}), release: make(chan struct{})}
	a.holds[strconv.FormatInt(userID, 10)] = hold
	return hold.started, func() { close(hold.release) }
}

func (a *fakeTelegram) setMember(userID int64, result string) {
	a.setRaw(userID, `{"ok":true,"result":`+result+`}`)
}

func (a *fakeTelegram) setRaw(userID int64, body string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.members == nil {
		a.members = make(map[string]string)
	}
	a.members[strconv.FormatInt(userID, 10)] = body
}

func (a *fakeTelegram) failSend() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sendErr = true
}

func (a *fakeTelegram) sentMessages() []sentMessage {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.sent)
}

func (a *fakeTelegram) memberChecks() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.checks)
}

type memberKey struct {
	chatID int64
	userID int64
}

// memStorage — хранилище в памяти: переживает пересоздание Handler и умеет падать.
type memStorage struct {
	rows          map[memberKey]string
	upsertErr     error
	deleteErr     error
	deleteChatErr error
	mu            sync.Mutex
}

func newMemStorage(users ...chat.TaggerUser) *memStorage {
	s := &memStorage{rows: make(map[memberKey]string)}
	for _, u := range users {
		s.rows[memberKey{chatID: u.ChatID, userID: u.UserID}] = u.Author
	}
	return s
}

func (s *memStorage) LoadAll(context.Context) ([]chat.TaggerUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.usersLocked(), nil
}

func (s *memStorage) Upsert(_ context.Context, u chat.TaggerUser) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.upsertErr != nil {
		return s.upsertErr
	}
	s.rows[memberKey{chatID: u.ChatID, userID: u.UserID}] = u.Author
	return nil
}

func (s *memStorage) Delete(_ context.Context, chatID, userID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deleteErr != nil {
		return s.deleteErr
	}
	delete(s.rows, memberKey{chatID: chatID, userID: userID})
	return nil
}

func (s *memStorage) DeleteChat(_ context.Context, chatID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deleteChatErr != nil {
		return s.deleteChatErr
	}
	for k := range s.rows {
		if k.chatID == chatID {
			delete(s.rows, k)
		}
	}
	return nil
}

func (s *memStorage) users() []chat.TaggerUser {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.usersLocked()
}

func (s *memStorage) usersLocked() []chat.TaggerUser {
	out := make([]chat.TaggerUser, 0, len(s.rows))
	for k, author := range s.rows {
		out = append(out, chat.TaggerUser{Author: author, ChatID: k.chatID, UserID: k.userID})
	}
	slices.SortFunc(out, func(a, b chat.TaggerUser) int {
		if a.ChatID != b.ChatID {
			return int(a.ChatID - b.ChatID)
		}
		return int(a.UserID - b.UserID)
	})
	return out
}

func (s *memStorage) fail(target *error, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	*target = err
}

type zeroRandom struct{}

func (zeroRandom) Intn(int) int       { return 0 }
func (zeroRandom) Int63n(int64) int64 { return 0 }

type fakeNicknames struct {
	err  error
	list []string
}

func (n *fakeNicknames) GetEnabledNicknames() ([]string, error) {
	return n.list, n.err
}

type fakeGenerator struct {
	onGenerate func()
	aiChances  []float32
}

func (g *fakeGenerator) GetMessageTextForTag(
	_ context.Context, _ []chathistory.Entry, aiChance float32, _ string,
) message.GenerationResult {
	g.aiChances = append(g.aiChances, aiChance)
	if g.onGenerate != nil {
		g.onGenerate()
	}
	return message.GenerationResult{
		Message:  "чего молчишь",
		Strategy: message.ByListGenerationStrategy,
	}
}

type noHistory struct{}

func (noHistory) Get(int64) []chathistory.Entry { return nil }

// fakeSettings — настройки чатов: теггер включён, пока чат не попал в disabled.
type fakeSettings struct {
	err      error
	disabled map[int64]bool
	aiChance float32
	mu       sync.Mutex
}

func (f *fakeSettings) GetForChat(_ context.Context, chatID int64) (*chatsettings.Settings, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return &chatsettings.Settings{TaggerEnabled: !f.disabled[chatID], AIChance: f.aiChance}, nil
}

func (f *fakeSettings) setEnabled(chatID int64, enabled bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.disabled == nil {
		f.disabled = make(map[int64]bool)
	}
	f.disabled[chatID] = !enabled
}

func (f *fakeSettings) fail(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

type noStats struct{}

func (noStats) Inc(context.Context, int64, int64, stats.OperationType, ...stats.Option) {}

type fakeLogger struct {
	warns []string
	mu    sync.Mutex
}

func (l *fakeLogger) WithError(ctx context.Context, _ error) context.Context { return ctx }

func (l *fakeLogger) WithFields(ctx context.Context, _ map[string]any) context.Context {
	return ctx
}

func (l *fakeLogger) Warn(_ context.Context, msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warns = append(l.warns, msg)
}

func (l *fakeLogger) messages() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.warns)
}

type stand struct {
	h        *Handler
	settings *fakeSettings
	api      *fakeTelegram
	bot      *telebot.Bot
	log      *fakeLogger
	gen      *fakeGenerator
	nicks    *fakeNicknames
}

// newRegistry поднимает реестр кандидатов из хранилища, как это делает main.
func newRegistry(t *testing.T, storage taggerusers.Store) *taggerusers.Registry {
	t.Helper()
	users, err := taggerusers.New(context.Background(), storage)
	require.NoError(t, err)
	return users
}

func newStand(t *testing.T, storage taggerusers.Store) *stand {
	t.Helper()
	return newStandWithAPI(t, storage, &fakeTelegram{})
}

// newStandWithAPI собирает Handler без фоновых горутин: тесты двигают планировщик
// вручную через tick.
func newStandWithAPI(t *testing.T, storage taggerusers.Store, api *fakeTelegram) *stand {
	t.Helper()

	server := httptest.NewServer(api)
	t.Cleanup(server.Close)
	bot, err := telebot.NewBot(telebot.Settings{
		Token: "test", URL: server.URL, Offline: true, Client: server.Client(),
	})
	require.NoError(t, err)
	bot.Me = &telebot.User{ID: botID, IsBot: true}

	s := &stand{
		api:      api,
		bot:      bot,
		log:      &fakeLogger{},
		gen:      &fakeGenerator{},
		nicks:    &fakeNicknames{list: []string{nick}},
		settings: &fakeSettings{},
	}
	s.h = newHandler(
		context.Background(), s.gen, noHistory{}, s.nicks, newRegistry(t, storage), bot, s.log,
		zeroRandom{}, noStats{}, s.settings, tagFrom, tagTo,
	)
	s.h.scheduleKnownChats()

	return s
}

func (s *stand) message(chatID int64, from *telebot.User) telebot.Context {
	return s.bot.NewContext(telebot.Update{Message: &telebot.Message{
		Chat:   &telebot.Chat{ID: chatID, Type: telebot.ChatSuperGroup},
		Sender: from,
		Text:   "привет",
	}})
}

func (s *stand) joined(chatID int64, from, joined *telebot.User) telebot.Context {
	return s.bot.NewContext(telebot.Update{Message: &telebot.Message{
		Chat:       &telebot.Chat{ID: chatID, Type: telebot.ChatSuperGroup},
		Sender:     from,
		UserJoined: joined,
	}})
}

// left — событие выхода в groupA.
func (s *stand) left(from, left *telebot.User) telebot.Context {
	return s.bot.NewContext(telebot.Update{Message: &telebot.Message{
		Chat:     &telebot.Chat{ID: groupA, Type: telebot.ChatSuperGroup},
		Sender:   from,
		UserLeft: left,
	}})
}

// due — момент, когда любая назначенная задача уже наступила.
func due() time.Time {
	return time.Now().Add(tagTo + time.Minute)
}

// jobsOf возвращает задачи чата в очереди.
func (s *stand) jobsOf(chatID int64) []taggerJob {
	s.h.mu.Lock()
	defer s.h.mu.Unlock()
	var out []taggerJob
	for _, j := range s.h.queue {
		if j.chatID == chatID {
			out = append(out, j)
		}
	}
	return out
}

func (s *stand) candidates(chatID int64) map[int64]string {
	s.h.mu.Lock()
	defer s.h.mu.Unlock()
	out := make(map[int64]string)
	for _, id := range s.h.users.Users(chatID) {
		out[id], _ = s.h.users.Author(chatID, id)
	}
	return out
}

func user(id int64, username string) *telebot.User {
	return &telebot.User{ID: id, Username: username}
}
