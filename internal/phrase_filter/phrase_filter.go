package phrase_filter

import (
	"regexp"
	"strings"
	"unicode"
)

// DefaultPhraseFilter реализует PhraseFilter с базовыми правилами определения осмысленности
type DefaultPhraseFilter struct {
	// Минимальная длина фразы для считания её осмысленной
	minLength int
	// Максимальная длина фразы (для отсечения слишком длинных сообщений)
	maxLength int
	// Минимальное количество слов
	minWords int
	// Регулярные выражения для определения неосмысленных паттернов
	meaninglessPatterns []*regexp.Regexp
	// Регулярные выражения для определения осмысленных паттернов
	meaningfulPatterns []*regexp.Regexp
}

// NewDefaultPhraseFilter создает новый экземпляр фильтра с настройками по умолчанию
func NewDefaultPhraseFilter() *DefaultPhraseFilter {
	return &DefaultPhraseFilter{
		minLength: 3,
		maxLength: 1000,
		minWords:  2,
		meaninglessPatterns: []*regexp.Regexp{
			regexp.MustCompile(`^[^\p{L}]*$`),                     // Только символы без букв
			regexp.MustCompile(`^[а-яё]{1,2}$`),                   // Одно-двухбуквенные слова
			regexp.MustCompile(`^[a-z]{1,2}$`),                    // Одно-двухбуквенные английские слова
			regexp.MustCompile(`^[^\p{L}]*[а-яё]{1,2}[^\p{L}]*$`), // Только одно-двухбуквенные слова с символами
			regexp.MustCompile(`^[^\p{L}]*[a-z]{1,2}[^\p{L}]*$`),  // Только одно-двухбуквенные английские слова с символами
			regexp.MustCompile(`^[а-яё]+[^\p{L}\d]*$`),            // Только русские буквы с символами (но не цифрами) в конце
			regexp.MustCompile(`^[a-z]+[^\p{L}\d]*$`),             // Только английские буквы с символами (но не цифрами) в конце
			regexp.MustCompile(`^[^\p{L}]*[а-яё]+$`),              // Только русские буквы с символами в начале
			regexp.MustCompile(`^[^\p{L}]*[a-z]+$`),               // Только английские буквы с символами в начале
		},
		meaningfulPatterns: []*regexp.Regexp{
			regexp.MustCompile(`\b[а-яё]{3,}\b`),         // Русские слова от 3 букв
			regexp.MustCompile(`\b[a-z]{3,}\b`),          // Английские слова от 3 букв
			regexp.MustCompile(`[а-яё]{3,}.*[а-яё]{3,}`), // Два русских слова от 3 букв
			regexp.MustCompile(`[a-z]{3,}.*[a-z]{3,}`),   // Два английских слова от 3 букв
		},
	}
}

// NewCustomPhraseFilter создает фильтр с пользовательскими настройками
func NewCustomPhraseFilter(minLength, maxLength, minWords int) *DefaultPhraseFilter {
	filter := NewDefaultPhraseFilter()
	filter.minLength = minLength
	filter.maxLength = maxLength
	filter.minWords = minWords
	return filter
}

// IsMeaningfulPhrase проверяет, является ли строка осмысленной фразой
func (f *DefaultPhraseFilter) IsMeaningfulPhrase(text string) bool {
	// Нормализуем текст
	normalizedText := strings.TrimSpace(text)

	// Проверяем наличие вопросительных или восклицательных конструкций
	// (эти фразы считаются осмысленными даже если они короткие)
	if f.containsQuestionOrExclamation(normalizedText) {
		return true
	}

	// Проверяем паттерны неосмысленности
	if f.matchesMeaninglessPatterns(normalizedText) {
		return false
	}

	// Проверяем базовые условия
	if !f.checkBasicConditions(normalizedText) {
		return false
	}

	// Проверяем паттерны осмысленности
	if f.matchesMeaningfulPatterns(normalizedText) {
		return true
	}

	// Дополнительные проверки
	return f.additionalChecks(normalizedText)
}

// checkBasicConditions проверяет базовые условия (длина, количество слов)
func (f *DefaultPhraseFilter) checkBasicConditions(text string) bool {
	// Проверяем длину
	if len(text) < f.minLength || len(text) > f.maxLength {
		return false
	}

	// Подсчитываем слова
	words := f.countWords(text)
	if words < f.minWords {
		return false
	}

	return true
}

// countWords подсчитывает количество слов в тексте
func (f *DefaultPhraseFilter) countWords(text string) int {
	words := strings.Fields(text)
	count := 0

	for _, word := range words {
		// Убираем пунктуацию и проверяем, что остались буквы или цифры
		cleanWord := strings.TrimFunc(word, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsNumber(r)
		})

		if len(cleanWord) >= 2 {
			count++
		}
	}

	return count
}

// matchesMeaninglessPatterns проверяет, соответствует ли текст паттернам неосмысленности
func (f *DefaultPhraseFilter) matchesMeaninglessPatterns(text string) bool {
	for _, pattern := range f.meaninglessPatterns {
		if pattern.MatchString(strings.ToLower(text)) {
			return true
		}
	}
	return false
}

// matchesMeaningfulPatterns проверяет, соответствует ли текст паттернам осмысленности
func (f *DefaultPhraseFilter) matchesMeaningfulPatterns(text string) bool {
	for _, pattern := range f.meaningfulPatterns {
		if pattern.MatchString(strings.ToLower(text)) {
			return true
		}
	}
	return false
}

// additionalChecks выполняет дополнительные проверки
func (f *DefaultPhraseFilter) additionalChecks(text string) bool {
	// Проверяем наличие глаголов (признак осмысленности)
	if f.containsVerbs(text) {
		return true
	}

	// Проверяем наличие числительных (часто указывает на осмысленность)
	if f.containsNumbers(text) {
		return true
	}

	return false
}

// containsVerbs проверяет наличие глаголов в тексте
func (f *DefaultPhraseFilter) containsVerbs(text string) bool {
	// Простая проверка на наличие глагольных окончаний
	verbPatterns := []*regexp.Regexp{
		regexp.MustCompile(`\b[а-яё]+(ть|л|ла|ло|ли|ю|ешь|ет|ем|ете|ут|ют|ишь|ит|им|ите|ат|ят)\b`),
		regexp.MustCompile(`\b[a-z]+(ing|ed|s|es)\b`),
	}

	for _, pattern := range verbPatterns {
		if pattern.MatchString(strings.ToLower(text)) {
			return true
		}
	}

	return false
}

// containsQuestionOrExclamation проверяет наличие вопросительных или восклицательных конструкций
func (f *DefaultPhraseFilter) containsQuestionOrExclamation(text string) bool {
	// Проверяем наличие вопросительных слов
	questionWords := []string{"что", "как", "где", "когда", "почему", "зачем", "what", "how", "where", "when", "why"}
	for _, word := range questionWords {
		if strings.Contains(strings.ToLower(text), word) {
			return true
		}
	}

	// Проверяем наличие вопросительных или восклицательных знаков
	// но только если фраза содержит осмысленные слова
	if (strings.Contains(text, "?") || strings.Contains(text, "!")) && f.hasMeaningfulWords(text) {
		return true
	}

	return false
}

// hasMeaningfulWords проверяет, содержит ли текст осмысленные слова
func (f *DefaultPhraseFilter) hasMeaningfulWords(text string) bool {
	words := strings.Fields(text)
	for _, word := range words {
		// Убираем пунктуацию и проверяем, что остались буквы
		cleanWord := strings.TrimFunc(word, func(r rune) bool {
			return !unicode.IsLetter(r)
		})

		// Слово считается осмысленным, если содержит 3 или более буквы
		if len(cleanWord) >= 3 {
			return true
		}
	}
	return false
}

// containsNumbers проверяет наличие чисел в тексте
func (f *DefaultPhraseFilter) containsNumbers(text string) bool {
	numberPattern := regexp.MustCompile(`\d+`)
	return numberPattern.MatchString(text)
}
