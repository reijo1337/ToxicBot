package vcs

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newRepo поднимает временный git-репозиторий с одним коммитом.
func newRepo(t *testing.T) Repo {
	t.Helper()

	dir := t.TempDir()

	for _, args := range [][]string{
		{"init", "-q", "-b", "master"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "test"},
	} {
		cmd := exec.CommandContext(t.Context(), "git", args...) //nolint:gosec // аргументы из теста
		cmd.Dir = dir
		require.NoError(t, cmd.Run(), "git %v", args)
	}

	require.NoError(t, os.WriteFile(filepath.Join(dir, "base.txt"), []byte("base"), 0o600))
	commit(t, dir, "base commit")

	return Repo{Dir: dir}
}

func commit(t *testing.T, dir, message string) {
	t.Helper()

	for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", message}} {
		cmd := exec.CommandContext(t.Context(), "git", args...) //nolint:gosec // аргументы из теста
		cmd.Dir = dir
		require.NoError(t, cmd.Run(), "git %v", args)
	}
}

func TestChangedFilesAndCommitBodies(t *testing.T) {
	t.Parallel()

	r := newRepo(t)
	ctx := context.Background()

	head, err := r.MergeBase(ctx, "HEAD")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(r.Dir, "next.txt"), []byte("next"), 0o600))
	commit(t, r.Dir, "second commit\n\n[skip-spec: хотфикс на проде]")

	changed, err := r.ChangedFiles(ctx, head)
	require.NoError(t, err)
	assert.Equal(t, []string{"next.txt"}, changed)

	bodies, err := r.CommitBodies(ctx, head)
	require.NoError(t, err)
	require.Len(t, bodies, 1)

	assert.Equal(t, []string{"хотфикс на проде"}, SkipReasons(bodies))
}

func TestSkipReasons(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		bodies []string
		want   []string
	}{
		"причина указана": {
			bodies: []string{"fix\n\n[skip-spec: срочно]"},
			want:   []string{"срочно"},
		},
		"пустая причина": {bodies: []string{"fix\n\n[skip-spec:]"}, want: []string{""}},
		"без обхода":     {bodies: []string{"обычный коммит"}, want: nil},
		"несколько обходов": {
			bodies: []string{"a\n\n[skip-spec: раз]", "b\n\n[skip-spec: два]"},
			want:   []string{"раз", "два"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, SkipReasons(tt.bodies))
		})
	}
}

func TestSkipReasons_OnlyStandaloneDirectiveCounts(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		body string
		want []string
	}{
		"директива отдельной строкой": {
			body: "fix: чиню\n\n[skip-spec: горит прод]",
			want: []string{"горит прод"},
		},
		"упоминание в предложении не считается": {
			body: "docs: описал, что [skip-spec: причина] понижает находку до предупреждения",
			want: nil,
		},
		"директива с отступом": {
			body: "fix\n\n  [skip-spec: причина]  ",
			want: []string{"причина"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, SkipReasons([]string{tt.body}))
		})
	}
}
