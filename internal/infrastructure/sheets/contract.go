//go:generate mockgen -source $GOFILE -destination mocks_test.go -package ${GOPACKAGE}
package sheets

import "gopkg.in/Iwark/spreadsheet.v2"

type sheets interface {
	GetSpreadsheet() (spreadsheet.Spreadsheet, error)
}
