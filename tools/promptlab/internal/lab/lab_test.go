package lab

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/reijo1337/ToxicBot/internal/features/chathistory"
	"github.com/reijo1337/ToxicBot/tools/promptlab/internal/cases"
	"github.com/reijo1337/ToxicBot/tools/promptlab/internal/conf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testEnvName = "PROMPTLAB_TEST_KEY"

func TestApplyBotHistory(t *testing.T) {
	t.Parallel()

	h := []chathistory.Entry{
		{ID: 1, Author: "@a", Text: "раз"},
		{ID: 2, Author: "бот", Text: "два", FromBot: true},
		{ID: 3, Author: "@a", Text: "три"},
		{ID: 4, Author: "бот", Text: "четыре", FromBot: true},
		{ID: 5, Author: "@a", Text: "пять"},
	}

	ids := func(es []chathistory.Entry) []int {
		out := make([]int, 0, len(es))
		for _, e := range es {
			out = append(out, e.ID)
		}
		return out
	}

	assert.Equal(t, []int{1, 2, 3, 4, 5}, ids(applyBotHistory(h, BotHistoryAll)))
	assert.Equal(t, []int{1, 3, 5}, ids(applyBotHistory(h, BotHistoryNone)))
	assert.Equal(t, []int{1, 3, 4, 5}, ids(applyBotHistory(h, BotHistoryLast)))

	_, err := ParseBotHistory("some")
	require.Error(t, err)
}

func TestRunTagUsesProductionTask(t *testing.T) {
	// Removing tag dispatch would send a reply envelope and the manual steering instead.
	var requests []completionRequest
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var request completionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		requests = append(requests, request)
		mu.Unlock()
		_, _ = io.WriteString(
			w,
			`{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"Самостоятельная синтетическая шутка."}}]}`,
		)
	}))
	defer server.Close()
	envName := testEnvName
	t.Setenv(envName, "synthetic-key")
	for _, mode := range []BotHistory{BotHistoryAll, BotHistoryNone, BotHistoryLast} {
		got, err := Run(t.Context(), Params{
			Cases: []cases.Case{
				{
					ID:   "tag-case",
					Kind: cases.KindTag,
					Target: &cases.Target{
						UserID:   901,
						Author:   "@synthetic_target",
						Nickname: "Чайник",
					},
					History: []cases.Entry{
						{ID: 1, SenderID: 901, Author: "@old_target", Text: "OWN_MARKER"},
						{ID: 2, SenderID: 902, Author: "@other", Text: "BACKGROUND_MARKER"},
						{ID: 3, FromBot: true, Text: "BOT_MUST_BE_EXCLUDED"},
					},
				},
			},
			Variants: []Variant{{Name: "baseline"}},
			Models: []conf.Model{
				{
					Name:      "test",
					Model:     "synthetic",
					APIKeyEnv: envName,
					BaseURL:   server.URL,
				},
			},
			Examples: []string{
				"Синтетическая шутка.",
			},
			BotHistory:  mode,
			Steering:    "MANUAL_STEERING_MUST_NOT_REPLACE_TAG",
			Repetitions: 1,
		})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Empty(t, got[0].Err)
	}
	require.Len(t, requests, 3)
	for _, request := range requests {
		require.Len(t, request.Messages, 2)
		assert.Equal(t, "system", request.Messages[0].Role)
		assert.Equal(t, "user", request.Messages[1].Role)
		task := request.Messages[1].Content
		assert.Contains(t, task, `event="timer"`)
		assert.Contains(t, task, `user_id="901"`)
		assert.Contains(t, task, "OWN_MARKER")
		assert.Contains(t, task, "BACKGROUND_MARKER")
		assert.NotContains(t, task, "BOT_MUST_BE_EXCLUDED")
		assert.NotContains(t, request.Messages[0].Content, "MANUAL_STEERING_MUST_NOT_REPLACE_TAG")
	}
	assert.Equal(t, requests[0], requests[1])
	assert.Equal(t, requests[0], requests[2])
}

func TestRunRejectsInvalidTagBeforeAPI(t *testing.T) {
	t.Parallel()
	tests := map[string]*cases.Target{
		"missing":  nil,
		"zero":     {},
		"negative": {UserID: -901},
	}
	for name, target := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := Run(t.Context(), Params{
				Cases: []cases.Case{
					{ID: "invalid-target", Kind: cases.KindTag, Target: target},
				},
				Variants: []Variant{{Name: "baseline"}},
				Models: []conf.Model{
					{Name: "test", APIKeyEnv: "PROMPTLAB_MISSING_INVALID_TARGET_KEY"},
				},
				Examples:    []string{"Синтетическая шутка."},
				Repetitions: 1,
			})
			require.ErrorContains(t, err, "target")
			assert.NotContains(t, err.Error(), "PROMPTLAB_MISSING_INVALID_TARGET_KEY")
		})
	}
}

func TestRunRepetitionsPreservesErrorsAndAnswers(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		finish := "stop"
		if calls.Add(1) == 2 {
			finish = "length"
		}
		_, _ = fmt.Fprintf(
			w,
			`{"choices":[{"finish_reason":%q,"message":{"role":"assistant","content":"Синтетический ответ %d."}}]}`,
			finish,
			calls.Load(),
		)
	}))
	defer server.Close()
	envName := testEnvName
	t.Setenv(envName, "synthetic-key")
	got, err := Run(t.Context(), Params{
		Cases: []cases.Case{
			{
				ID:      "first",
				Kind:    cases.KindText,
				History: []cases.Entry{{ID: 1, Author: "@synthetic", Text: "Первая тема"}},
			},
			{
				ID:      "second",
				Kind:    cases.KindText,
				History: []cases.Entry{{ID: 1, Author: "@synthetic", Text: "Вторая тема"}},
			},
		},
		Variants: []Variant{{Name: "baseline"}},
		Models: []conf.Model{
			{
				Name:      "test",
				Model:     "synthetic",
				APIKeyEnv: envName,
				BaseURL:   server.URL,
			},
		},
		Examples:    []string{"Синтетическая шутка."},
		Repetitions: 3,
	})
	require.NoError(t, err)
	require.Len(t, got, 6)
	assert.Equal(t, int64(6), calls.Load())
	assert.Equal(
		t,
		[]int{1, 2, 3, 1, 2, 3},
		[]int{
			got[0].Repetition,
			got[1].Repetition,
			got[2].Repetition,
			got[3].Repetition,
			got[4].Repetition,
			got[5].Repetition,
		},
	)
	assert.Equal(t, "first", got[0].CaseID)
	assert.Equal(t, "second", got[3].CaseID)
	assert.Contains(t, got[1].Err, `finish_reason="length"`)
	assert.Empty(t, got[1].Text)
	assert.Contains(t, got[0].Text, "ответ 1")
	assert.Contains(t, got[2].Text, "ответ 3")
}

type completionRequest struct {
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

func TestRunNegativeTagFixturesFallsBackWithoutAPI(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(
			w,
			`{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"AI не должен вызываться."}}]}`,
		)
	}))
	defer server.Close()
	envName := testEnvName
	t.Setenv(envName, "synthetic-key")
	fixtures, err := cases.Load("../../testdata/tag-attribution-negative.json")
	require.NoError(t, err)
	require.Len(t, fixtures, 2)
	got, err := Run(t.Context(), Params{
		Cases:    fixtures,
		Variants: []Variant{{Name: "baseline"}},
		Models: []conf.Model{
			{Name: "test", Model: "synthetic", APIKeyEnv: envName, BaseURL: server.URL},
		},
		Examples:    []string{"Синтетическая фраза из списка."},
		Repetitions: 1,
	})
	require.NoError(t, err)
	require.Len(t, got, 2)
	for i, result := range got {
		assert.Equal(t, fixtures[i].ID, result.CaseID)
		assert.Contains(t, result.Err, "фоллбэк на список без ошибки LLM")
		assert.Empty(t, result.Text, "list fallback is not an AI acceptance result")
	}
	assert.Zero(t, calls.Load(), "absent and legacy-only targets must not reach the API")
}
