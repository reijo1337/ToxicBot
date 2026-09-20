// Package conf читает список моделей для прогона из promptlab.yaml.
package conf

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Model — одна OpenAI-совместимая точка входа; ключ берётся из окружения, в конфиг не попадает.
type Model struct {
	Name      string `yaml:"name"`
	BaseURL   string `yaml:"base_url"`
	APIKeyEnv string `yaml:"api_key_env"`
	Model     string `yaml:"model"`
	// Temperature — как DEEPSEEK_TEMPERATURE в проде.
	Temperature float64 `yaml:"temperature"`
	// ExtraBody уходит в тело запроса как есть (thinking: {type: disabled} для DeepSeek).
	ExtraBody map[string]any `yaml:"extra_body"`
	// MaxTokens переопределяет общий лимит для модели, у которой reasoning не отключается.
	MaxTokens int64 `yaml:"max_tokens"`
}

type Config struct {
	Models []Model `yaml:"models"`
	// MaxTokens — как DEEPSEEK_MAX_TOKENS в проде.
	MaxTokens int64 `yaml:"max_tokens"`
	// Concurrency — сколько пар вариант × модель гонять параллельно.
	Concurrency int `yaml:"concurrency"`
}

func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // путь задаёт пользователь инструмента
	if err != nil {
		return Config{}, fmt.Errorf("прочитать %s: %w", path, err)
	}

	var c Config
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return Config{}, fmt.Errorf("разобрать %s: %w", path, err)
	}

	if len(c.Models) == 0 {
		return Config{}, errors.New("в конфиге нет ни одной модели")
	}
	seen := make(map[string]struct{}, len(c.Models))
	for i, m := range c.Models {
		if m.Name == "" || m.BaseURL == "" || m.APIKeyEnv == "" || m.Model == "" {
			return Config{}, fmt.Errorf(
				"модель #%d: name, base_url, api_key_env и model обязательны",
				i+1,
			)
		}
		if _, dup := seen[m.Name]; dup {
			return Config{}, fmt.Errorf("модель %q встречается дважды", m.Name)
		}
		seen[m.Name] = struct{}{}
	}
	if c.MaxTokens <= 0 {
		c.MaxTokens = 500
	}
	if c.Concurrency <= 0 {
		c.Concurrency = 4
	}
	return c, nil
}
