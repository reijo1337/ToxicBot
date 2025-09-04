package duckdb

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	"github.com/hashicorp/go-multierror"
	"github.com/jmoiron/sqlx"
	_ "github.com/marcboeker/go-duckdb/v2"
)

func init() {
	database.Register("duckdb", &DuckDB{})
}

const DefaultMigrationsTable = "schema_migrations"

type Config struct {
	MigrationsTable string
	NoTxWrap        bool
}

type DuckDB struct {
	db       *sqlx.DB
	isLocked atomic.Bool
	config   *Config
}

func WithInstance(instance *sqlx.DB, config *Config) (database.Driver, error) {
	if config == nil {
		return nil, errors.New("no config")
	}

	if err := instance.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	if len(config.MigrationsTable) == 0 {
		config.MigrationsTable = DefaultMigrationsTable
	}

	mx := &DuckDB{
		db:     instance,
		config: config,
	}
	if err := mx.ensureVersionTable(); err != nil {
		return nil, err
	}
	return mx, nil
}

func (d *DuckDB) ensureVersionTable() (err error) {
	if err = d.Lock(); err != nil {
		return err
	}

	defer func() {
		if e := d.Unlock(); e != nil {
			if err == nil {
				err = e
			} else {
				err = errors.Join(err, e)
			}
		}
	}()

	const queryFormat = `
create table if not exists %s (
	version bigint
	,dirty bool
)`
	query := fmt.Sprintf(queryFormat, d.config.MigrationsTable)

	if _, err := d.db.Exec(query); err != nil {
		return err
	}
	return nil
}

func (d *DuckDB) Open(dbURL string) (database.Driver, error) {
	parsedURL, err := url.Parse(dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse db url: %w", err)
	}

	dbfile := strings.Replace(migrate.FilterCustomQuery(parsedURL).String(), "duckdb://", "", 1)

	db, err := sqlx.Open("duckdb", dbfile)
	if err != nil {
		return nil, fmt.Errorf("failed to open database '%s': %w", dbfile, err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	qv := parsedURL.Query()

	migrationsTable := qv.Get("x-migrations-table")
	if len(migrationsTable) == 0 {
		migrationsTable = DefaultMigrationsTable
	}

	noTxWrap := false
	if v := qv.Get("x-no-tx-wrap"); v != "" {
		noTxWrap, err = strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("x-no-tx-wrap: %w", err)
		}
	}

	mx, err := WithInstance(db, &Config{
		MigrationsTable: migrationsTable,
		NoTxWrap:        noTxWrap,
	})
	if err != nil {
		return nil, err
	}
	return mx, nil
}

func (d *DuckDB) Close() error { return d.db.Close() }

func (d *DuckDB) Lock() error {
	if !d.isLocked.CompareAndSwap(false, true) {
		return database.ErrLocked
	}
	return nil
}

func (d *DuckDB) Unlock() error {
	if !d.isLocked.CompareAndSwap(true, false) {
		return database.ErrNotLocked
	}
	return nil
}

func (d *DuckDB) Run(migration io.Reader) error {
	migr, err := io.ReadAll(migration)
	if err != nil {
		return err
	}
	query := string(migr)

	if d.config.NoTxWrap {
		return d.executeQueryNoTx(query)
	}
	return d.executeQuery(query)
}

func (m *DuckDB) executeQuery(query string) error {
	tx, err := m.db.Begin()
	if err != nil {
		return &database.Error{OrigErr: err, Err: "transaction start failed"}
	}
	if _, err := tx.Exec(query); err != nil {
		if errRollback := tx.Rollback(); errRollback != nil {
			err = errors.Join(err, errRollback)
		}
		return &database.Error{OrigErr: err, Query: []byte(query)}
	}
	if err := tx.Commit(); err != nil {
		return &database.Error{OrigErr: err, Err: "transaction commit failed"}
	}
	return nil
}

func (m *DuckDB) executeQueryNoTx(query string) error {
	if _, err := m.db.Exec(query); err != nil {
		return &database.Error{OrigErr: err, Query: []byte(query)}
	}
	return nil
}

func (d *DuckDB) SetVersion(version int, dirty bool) error {
	tx, err := d.db.Begin()
	if err != nil {
		return &database.Error{OrigErr: err, Err: "transaction start failed"}
	}

	query := "delete from " + d.config.MigrationsTable //nolint: gosec
	if _, err := tx.Exec(query); err != nil {
		return &database.Error{OrigErr: err, Query: []byte(query)}
	}

	// Also re-write the schema version for nil dirty versions to prevent
	// empty schema version for failed down migration on the first migration
	// See: https://github.com/golang-migrate/migrate/issues/330
	if version >= 0 || (version == database.NilVersion && dirty) {
		//nolint: gosec
		query := fmt.Sprintf(
			`INSERT INTO %s (version, dirty) VALUES ($1::bigint, $2::bool)`,
			d.config.MigrationsTable,
		)
		if _, err := tx.Exec(query, version, dirty); err != nil {
			if errRollback := tx.Rollback(); errRollback != nil {
				err = multierror.Append(err, errRollback)
			}
			return &database.Error{OrigErr: err, Query: []byte(query)}
		}
	}

	if err := tx.Commit(); err != nil {
		return &database.Error{OrigErr: err, Err: "transaction commit failed"}
	}

	return nil
}

func (d *DuckDB) Version() (version int, dirty bool, err error) {
	query := "SELECT version, dirty FROM " + d.config.MigrationsTable + " LIMIT 1"
	err = d.db.QueryRow(query).Scan(&version, &dirty)
	if err != nil {
		return database.NilVersion, false, nil
	}
	return version, dirty, nil
}

func (d *DuckDB) Drop() error {
	allTablesQuery := `
	select
		table_name
	from
		information_schema.tables
	where
		table_schema = (select current_schema())
		and
		table_type='BASE TABLE'`

	tableNames := []string{}
	if err := d.db.Select(&tableNames, allTablesQuery); err != nil {
		return &database.Error{OrigErr: err, Query: []byte(allTablesQuery)}
	}

	if len(tableNames) > 0 {
		for _, tableName := range tableNames {
			query := "drop table if exists " + tableName
			if tableName == d.config.MigrationsTable {
				query = "delete from " + tableName
			}
			if _, err := d.db.Exec(query); err != nil {
				return &database.Error{OrigErr: err, Query: []byte(query)}
			}
		}

		if _, err := d.db.Exec("vacuum"); err != nil {
			return &database.Error{OrigErr: err, Query: []byte("vacuum")}
		}

		if _, err := d.db.Exec("CHECKPOINT"); err != nil {
			return &database.Error{OrigErr: err, Query: []byte("CHECKPOINT")}
		}

		time.Sleep(time.Second)
	}

	return nil
}
