package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReady(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		dirs      []string
		files     []string
		wantError string
	}{
		{name: "changes directory is absent"},
		{name: "changes directory is empty", dirs: []string{""}},
		{name: "service files are allowed", files: []string{".gitkeep"}},
		{name: "archive is not active", dirs: []string{"archive/2026-01-01-old"}},
		{
			name:      "lite change without a delta blocks merge",
			dirs:      []string{"fix-config"},
			wantError: "fix-config",
		},
		{
			name:      "completed tasks still require archiving",
			files:     []string{"completed/tasks.md"},
			wantError: "completed",
		},
		{
			name:      "all active changes are listed",
			dirs:      []string{"second", "first", "archive/2026-01-01-old"},
			wantError: "first, second",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := readyRoot(t, tt.dirs, tt.files)
			err := run([]string{"ready", "--root", root})
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("ready: %v", err)
				}

				return
			}

			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("ready error = %v, want changes %q", err, tt.wantError)
			}

			if !strings.Contains(err.Error(), "make spec-archive name=<id>") {
				t.Fatalf("ready error has no archive instruction: %v", err)
			}
		})
	}
}

func readyRoot(t *testing.T, dirs, files []string) string {
	t.Helper()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "openspec", "specs"), 0o750); err != nil {
		t.Fatal(err)
	}

	changesDir := filepath.Join(root, "openspec", "changes")
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(changesDir, dir), 0o750); err != nil {
			t.Fatal(err)
		}
	}

	for _, file := range files {
		path := filepath.Join(changesDir, file)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, []byte("- [x] done\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	return root
}
