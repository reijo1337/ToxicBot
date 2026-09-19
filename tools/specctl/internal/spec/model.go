// Package spec разбирает living specs и метаданные capability.
package spec

// Coverage — как сценарий доказывается.
type Coverage string

const (
	// CoverageCovered — сценарий закрыт автотестом с якорем "// spec: <ID>".
	CoverageCovered Coverage = "covered"
	// CoverageManual — проверяется руками, причина обязательна в notes.
	CoverageManual Coverage = "manual"
	// CoverageTodo — технический долг, учитывается храповиком.
	CoverageTodo Coverage = "todo"
)

// Valid сообщает, известно ли значение покрытия.
func (c Coverage) Valid() bool {
	switch c {
	case CoverageCovered, CoverageManual, CoverageTodo:
		return true
	default:
		return false
	}
}

// Scenario — один сценарий living spec.
type Scenario struct {
	ID          string
	Title       string
	Requirement string
	Line        int
}
