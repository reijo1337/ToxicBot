package db

import (
	"database/sql/driver"
	"fmt"

	jsonlib "github.com/goccy/go-json"
)

type json interface {
	*responseLogExtra | map[string]uint64
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

	var b []byte
	switch v := value.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return fmt.Errorf("unsupported type for JSONMap: %T", value)
	}

	if len(b) == 0 {
		return nil
	}

	tmp := new(T)
	err := jsonlib.Unmarshal(b, tmp)
	a.t = *tmp

	return err
}
