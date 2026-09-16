// Package check содержит проверки, связывающие спеки, код и тесты.
package check

import "fmt"

// Severity — вес находки.
type Severity int

const (
	// SeverityWarn не роняет проверку, но попадает в отчёт.
	SeverityWarn Severity = iota
	// SeverityFail роняет проверку и блокирует сборку.
	SeverityFail
)

// String печатает вес находки для вывода в консоль.
func (s Severity) String() string {
	if s == SeverityFail {
		return "FAIL"
	}

	return "WARN"
}

// Finding — одна находка проверки.
type Finding struct {
	Rule     string
	Message  string
	Location string
	Severity Severity
}

// String собирает строку для человека.
func (f Finding) String() string {
	if f.Location == "" {
		return fmt.Sprintf("%s [%s] %s", f.Severity, f.Rule, f.Message)
	}

	return fmt.Sprintf("%s [%s] %s: %s", f.Severity, f.Rule, f.Location, f.Message)
}

// HasFailures сообщает, есть ли среди находок блокирующие.
func HasFailures(findings []Finding) bool {
	for _, f := range findings {
		if f.Severity == SeverityFail {
			return true
		}
	}

	return false
}
