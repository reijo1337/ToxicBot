package google_spreadsheet

import "gopkg.in/Iwark/spreadsheet.v2"

type Manager interface {
	GetSpreadsheet() (spreadsheet.Spreadsheet, error)
}
