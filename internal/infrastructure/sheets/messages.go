package sheets

import (
	"fmt"
	"strings"
)

type Repository struct {
	sheets sheets
}

func New(sheets sheets) *Repository {
	return &Repository{
		sheets: sheets,
	}
}

func (r *Repository) GetEnabledGreetings() ([]string, error) {
	return getEditableMessagesFromSheet(r.sheets, "greetings")
}

func (r *Repository) GetEnabledStickers() ([]string, error) {
	return getEditableMessagesFromSheet(r.sheets, "stickers")
}

func (r *Repository) GetEnabledVoices() ([]string, error) {
	return getEditableMessagesFromSheet(r.sheets, "voice")
}

func (r *Repository) GetEnabledRandom() ([]string, error) {
	return getEditableMessagesFromSheet(r.sheets, "random")
}

func (r *Repository) GetEnabledNicknames() ([]string, error) {
	return getEditableMessagesFromSheet(r.sheets, "nickname")
}

func getEditableMessagesFromSheet(sheets sheets, name string) ([]string, error) {
	spreadsheet, err := sheets.GetSpreadsheet()
	if err != nil {
		return nil, fmt.Errorf("can't get spreadsheet")
	}

	sheet, err := spreadsheet.SheetByTitle(name)
	if err != nil {
		return nil, fmt.Errorf("can't get sheet %s: %w", name, err)
	}

	out := make([]string, 0, len(sheet.Rows))
	for _, row := range sheet.Rows[1:] {
		if strings.Contains(row[1].Value, "TRUE") {
			out = append(out, row[0].Value)
		}
	}

	return out, nil
}
