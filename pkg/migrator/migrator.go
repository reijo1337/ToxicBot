package migrator

import (
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/reijo1337/ToxicBot/pkg/migrator/duckdb"
)

func MigrateDB(filepath string) error {
	m, err := migrate.New("file://./db/migrations", "duckdb://"+filepath)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}

	version, dirty, err := m.Version()
	if err != nil {
		if !errors.Is(err, migrate.ErrNilVersion) {
			return fmt.Errorf("failed to get current version: %w", err)
		}
	}

	if dirty {
		if version == 1 {
			if err := m.Drop(); err != nil {
				return fmt.Errorf("failed to drop: %w", err)
			}
		} else {
			if err := m.Force(int(version) - 1); err != nil { //nolint: gosec
				return fmt.Errorf("failed to force migrate: %w", err)
			}
		}
	}

	if err := m.Up(); err != nil {
		if !errors.Is(err, migrate.ErrNoChange) {
			return fmt.Errorf("failed to migrate: %w", err)
		}
	}

	return nil
}
