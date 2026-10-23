package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_InvalidEnvFails(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "unparsable_value",
			env: map[string]string{
				"TELEGRAM_TOKEN": "test", "SQLITE_FILE_PATH": "test.db", "PHOTO_REACTIONS_CHANCE": "abc",
			},
			want: "PHOTO_REACTIONS_CHANCE",
		},
		{
			name: "missing_required",
			env:  map[string]string{"SQLITE_FILE_PATH": "test.db"},
			want: "TELEGRAM_TOKEN",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// t.Setenv восстановит значение после теста, Unsetenv делает переменную отсутствующей.
			t.Setenv("TELEGRAM_TOKEN", "")
			require.NoError(t, os.Unsetenv("TELEGRAM_TOKEN"))
			for key, value := range tc.env {
				t.Setenv(key, value)
			}

			cfg, err := Parse()
			require.ErrorContains(t, err, tc.want)
			assert.Nil(t, cfg)
		})
	}
}
