package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reijo1337/ToxicBot/tools/promptlab/internal/report"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunMatrixDefaultRepetition(t *testing.T) {
	t.Setenv("PROMPTLAB_TEST_KEY", "synthetic-key")
	checkMatrixRepetitions(t, nil, 1)
}

func TestRunMatrixExplicitRepetitions(t *testing.T) {
	t.Setenv("PROMPTLAB_TEST_KEY", "synthetic-key")
	checkMatrixRepetitions(t, []string{"--repetitions", "2"}, 2)
}

func checkMatrixRepetitions(t *testing.T, extra []string, count int) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(
			w,
			`{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"Синтетический ответ стенда."}}]}`,
		)
	}))
	defer server.Close()
	dir := t.TempDir()
	write := func(name, content string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
		return path
	}
	config := write(
		"config.yaml",
		fmt.Sprintf(
			"models:\n  - name: synthetic\n    base_url: %s\n    api_key_env: PROMPTLAB_TEST_KEY\n    model: synthetic\n    temperature: 1.1\n",
			server.URL,
		),
	)
	fixture := write(
		"cases.json",
		`[{"id":"synthetic","kind":"text","history":[{"id":1,"author":"@synthetic","text":"тема"}]}]`,
	)
	examples := write("examples.txt", "Синтетическая шутка.\n")
	out := filepath.Join(dir, "report.md")
	jsonl := filepath.Join(dir, "results.jsonl")
	args := make([]string, 0, 12+len(extra))
	args = append(args,
		"--config",
		config,
		"--cases",
		fixture,
		"--examples",
		examples,
		"--prompts",
		filepath.Join(dir, "prompts"),
		"--out",
		out,
		"--jsonl",
		jsonl,
	)
	args = append(args, extra...)
	require.NoError(t, runMatrix(t.Context(), args))
	raw, err := os.ReadFile(jsonl) //nolint:gosec // path is inside t.TempDir
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	require.Len(t, lines, count)
	for i, line := range lines {
		var result report.Result
		require.NoError(t, json.Unmarshal([]byte(line), &result))
		assert.Equal(t, i+1, result.Repetition)
		assert.NotEmpty(t, result.Text)
	}
	md, err := os.ReadFile(out) //nolint:gosec // path is inside t.TempDir
	require.NoError(t, err)
	assert.Equal(t, count, strings.Count(string(md), "Синтетический ответ стенда."))
}

func TestRunMatrixRejectsNonpositiveRepetitions(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"0", "-1"} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			err := runMatrix(
				t.Context(),
				[]string{"--repetitions", value, "--config", "missing-config.yaml"},
			)
			require.ErrorContains(t, err, "repetitions")
		})
	}
}
