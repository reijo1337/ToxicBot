// Package conf читает настройки specctl из openspec/specctl.yaml.
package conf

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"gopkg.in/yaml.v3"
)

// Config — настройки гейта, лежащие рядом с контуром.
type Config struct {
	// AllowedOrphans — пакеты, которым позволено не принадлежать ни одной capability.
	AllowedOrphans []string `yaml:"allowed_orphans"`
	// SkipDrift — пути, изменение которых никогда не считается изменением поведения.
	SkipDrift []string `yaml:"skip_drift"`
}

// Load читает конфигурацию; отсутствие файла не считается ошибкой.
func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // путь собирается из корня репозитория

	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}

	if err != nil {
		return Config{}, fmt.Errorf("читать %s: %w", path, err)
	}

	var c Config
	if err = yaml.Unmarshal(raw, &c); err != nil {
		return Config{}, fmt.Errorf("разобрать %s: %w", path, err)
	}

	return c, nil
}
