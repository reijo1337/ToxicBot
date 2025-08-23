package phrase_filter

import (
	"testing"
)

func TestDefaultPhraseFilter_IsMeaningfulPhrase(t *testing.T) {
	filter := NewDefaultPhraseFilter()

	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		// Осмысленные фразы
		{
			name:     "Простая осмысленная фраза",
			text:     "Привет, как дела?",
			expected: true,
		},
		{
			name:     "Фраза с глаголом",
			text:     "Я иду домой",
			expected: true,
		},
		{
			name:     "Вопросительная фраза",
			text:     "Что ты делаешь?",
			expected: true,
		},
		{
			name:     "Фраза с числами",
			text:     "Сегодня 15 марта",
			expected: true,
		},
		{
			name:     "Английская фраза",
			text:     "Hello, how are you?",
			expected: true,
		},
		{
			name:     "Фраза с восклицанием",
			text:     "Отлично!",
			expected: true,
		},
		{
			name:     "Длинная осмысленная фраза",
			text:     "Сегодня прекрасная погода для прогулки в парке",
			expected: true,
		},

		// Неосмысленные фразы
		{
			name:     "Одно слово",
			text:     "Привет",
			expected: false,
		},
		{
			name:     "Короткое слово",
			text:     "а",
			expected: false,
		},
		{
			name:     "Два коротких слова",
			text:     "а б",
			expected: false,
		},
		{
			name:     "Только символы",
			text:     "!!!",
			expected: false,
		},
		{
			name:     "Смешанные символы",
			text:     "а!",
			expected: false,
		},
		{
			name:     "Короткое английское слово",
			text:     "hi",
			expected: false,
		},
		{
			name:     "Пустая строка",
			text:     "",
			expected: false,
		},
		{
			name:     "Только пробелы",
			text:     "   ",
			expected: false,
		},
		{
			name:     "Очень длинная строка",
			text:     "очень длинная строка " + string(make([]rune, 1000)),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filter.IsMeaningfulPhrase(tt.text)
			if result != tt.expected {
				t.Errorf("IsMeaningfulPhrase(%q) = %v, want %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestCustomPhraseFilter_IsMeaningfulPhrase(t *testing.T) {
	// Создаем фильтр с более строгими требованиями
	filter := NewCustomPhraseFilter(5, 500, 3)

	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		{
			name:     "Короткая фраза (должна быть отклонена)",
			text:     "Привет всем",
			expected: false, // Меньше 5 символов
		},
		{
			name:     "Длинная фраза с тремя словами",
			text:     "Сегодня прекрасная погода",
			expected: true,
		},
		{
			name:     "Фраза с двумя словами (должна быть отклонена)",
			text:     "Привет всем",
			expected: false, // Только 2 слова
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filter.IsMeaningfulPhrase(tt.text)
			if result != tt.expected {
				t.Errorf("IsMeaningfulPhrase(%q) = %v, want %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestPhraseFilter_EdgeCases(t *testing.T) {
	filter := NewDefaultPhraseFilter()

	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		{
			name:     "Фраза с эмодзи",
			text:     "Привет! 😊",
			expected: true,
		},
		{
			name:     "Фраза с цифрами и буквами",
			text:     "Версия 2.0",
			expected: true,
		},
		{
			name:     "Фраза с пунктуацией",
			text:     "Да, конечно!",
			expected: true,
		},
		{
			name:     "Фраза с множественными пробелами",
			text:     "  Привет   всем  ",
			expected: true,
		},
		{
			name:     "Фраза с табуляцией",
			text:     "Привет\tвсем",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filter.IsMeaningfulPhrase(tt.text)
			if result != tt.expected {
				t.Errorf("IsMeaningfulPhrase(%q) = %v, want %v", tt.text, result, tt.expected)
			}
		})
	}
}

func BenchmarkDefaultPhraseFilter_IsMeaningfulPhrase(b *testing.B) {
	filter := NewDefaultPhraseFilter()
	text := "Сегодня прекрасная погода для прогулки в парке"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filter.IsMeaningfulPhrase(text)
	}
}
