package config

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func clearConfigEnv(t *testing.T) {
	t.Helper()
	configType := reflect.TypeFor[Config]()
	for i := range configType.NumField() {
		field := configType.Field(i)
		for _, key := range []string{strings.ToUpper(field.Name), field.Tag.Get("envconfig")} {
			t.Setenv(key, "")
			require.NoError(t, os.Unsetenv(key))
		}
	}
	t.Setenv("TELEGRAM_TOKEN", "test")
	t.Setenv("SQLITE_FILE_PATH", "test.db")
}

//nolint:paralleltest // clearConfigEnv changes process environment through t.Setenv.
func TestParse_PhotoChanceDefault(t *testing.T) {
	clearConfigEnv(t)
	cfg, err := Parse()
	require.NoError(t, err)
	assert.InDelta(t, 1, cfg.PhotoReactChance, 0.001)
}

func TestParse_PhotoChanceOverride(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  float32
	}{
		{"0", 0}, {"0.75", 0.75}, {"0.4", 0.4}, {"1", 1},
	} {
		t.Run(tc.value, func(t *testing.T) {
			clearConfigEnv(t)
			t.Setenv("PHOTO_REACTIONS_CHANCE", tc.value)
			cfg, err := Parse()
			require.NoError(t, err)
			assert.InDelta(t, tc.want, cfg.PhotoReactChance, 0.001)
		})
	}
}
