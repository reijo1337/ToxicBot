package db

import (
	"database/sql/driver"
	"errors"

	jsonlib "github.com/goccy/go-json"
)

type json interface {
	*responseLogExtra
}

type JSON[T json] struct {
	t T
}

func (a JSON[T]) Value() (driver.Value, error) {
	if a.t == nil {
		return nil, nil
	}
	data, err := jsonlib.Marshal(a.t)
	return string(data), err
}

func (a *JSON[T]) Scan(value any) error {
	if value == nil {
		return nil
	}

	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}

	if len(b) == 0 {
		return nil
	}

	tmp := new(T)
	err := jsonlib.Unmarshal(b, tmp)
	a.t = *tmp

	return err
}
