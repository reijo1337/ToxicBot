package db

import (
	"fmt"
	"time"
)

type nullDate struct {
	Time  time.Time
	Valid bool
}

func (n *nullDate) Scan(value any) error {
	if value == nil {
		n.Valid = false
		return nil
	}
	var s string
	switch v := value.(type) {
	case string:
		s = v
	case []byte:
		s = string(v)
	default:
		return fmt.Errorf("unsupported type for nullDate: %T", value)
	}
	if s == "" {
		n.Valid = false
		return nil
	}
	formats := []string{
		"2006-01-02",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05.999999-07:00",
		time.RFC3339,
	}
	var t time.Time
	var err error
	for _, layout := range formats {
		t, err = time.Parse(layout, s)
		if err == nil {
			break
		}
	}
	if err != nil {
		return fmt.Errorf("parse date %q: %w", s, err)
	}
	n.Time = t
	n.Valid = true
	return nil
}
