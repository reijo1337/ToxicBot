package storage

import (
	"fmt"
	"strings"

	"github.com/reijo1337/ToxicBot/internal/google_spreadsheet"
	"gopkg.in/Iwark/spreadsheet.v2"
)

type Storage struct {
	manager google_spreadsheet.Manager
}

type SheetNameType string

const (
	SheetNameGreetings SheetNameType = "greetings"
	SheetNameIgor      SheetNameType = "igor"
	SheetNameMax       SheetNameType = "max"
	SheetNameRandom    SheetNameType = "random"
	SheetNameStickers  SheetNameType = "stickers"
	SheetNameVoice     SheetNameType = "voice"
)

func (t SheetNameType) ToString() string {
	return string(t)
}

func New(manager google_spreadsheet.Manager) *Storage {
	return &Storage{
		manager,
	}
}

func (s *Storage) readSheet(sheetName SheetNameType) (*spreadsheet.Sheet, error) {
	ss, err := s.manager.GetSpreadsheet()
	if err != nil {
		return nil, fmt.Errorf("cannot get spreadsheet: %w", err)
	}

	sheet, err := ss.SheetByTitle(sheetName.ToString())
	if err != nil {
		return nil, fmt.Errorf("cannot get sheet %v: %w", sheetName.ToString(), err)
	}

	return sheet, nil
}

func getAllValuesInSheet[T DTOType](s *Storage, sheetName SheetNameType, rowConverter func(row []spreadsheet.Cell) *T) (rows []T, err error) {
	sheet, err := s.readSheet(sheetName)
	if err != nil {
		return nil, err
	}

	rows = make([]T, 0, len(sheet.Rows)-1)
	for _, row := range sheet.Rows[1:] {
		v := rowConverter(row)
		if v != nil {
			rows = append(rows, *v)
		}
	}
	return rows, nil
}

func (s *Storage) GetGreetings() (GreetingsDTOs, error) {
	return getAllValuesInSheet(s, SheetNameGreetings, func(row []spreadsheet.Cell) *GreetingsDTO {
		return &GreetingsDTO{
			Text:      row[0].Value,
			IsEnabled: strings.Contains(row[1].Value, "TRUE"),
		}
	})
}

func (s *Storage) GetIgors() (IgorDTOs, error) {
	return getAllValuesInSheet(s, SheetNameIgor, func(row []spreadsheet.Cell) *IgorDTO {
		return &IgorDTO{
			Text:      row[0].Value,
			IsEnabled: strings.Contains(row[1].Value, "TRUE"),
		}
	})
}

func (s *Storage) GetMaxs() (MaxDTOs, error) {
	return getAllValuesInSheet(s, SheetNameIgor, func(row []spreadsheet.Cell) *MaxDTO {
		return &MaxDTO{
			Text:      row[0].Value,
			IsEnabled: strings.Contains(row[1].Value, "TRUE"),
		}
	})
}

func (s *Storage) GetRandom() (RandomDTOs, error) {
	return getAllValuesInSheet(s, SheetNameRandom, func(row []spreadsheet.Cell) *RandomDTO {
		return &RandomDTO{
			Text:      row[0].Value,
			IsEnabled: strings.Contains(row[1].Value, "TRUE"),
		}
	})
}

func (s *Storage) GetStickers() (StikersDTOs, error) {
	return getAllValuesInSheet(s, SheetNameStickers, func(row []spreadsheet.Cell) *StikersDTO {
		return &StikersDTO{
			StickerID: row[0].Value,
			IsEnabled: strings.Contains(row[1].Value, "TRUE"),
		}
	})
}

func (s *Storage) GetVoices() (VoiceDTOs, error) {
	return getAllValuesInSheet(s, SheetNameVoice, func(row []spreadsheet.Cell) *VoiceDTO {
		return &VoiceDTO{
			VoiceID:   row[0].Value,
			IsEnabled: strings.Contains(row[1].Value, "TRUE"),
		}
	})
}
