// for future use github.com/avito-tech/go-transaction-manager/sqlx
package db

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"
)

type Execer interface {
	NamedExecContext(ctx context.Context, query string, arg interface{}) (sql.Result, error)
	SelectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
	GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
}

type connGetter interface {
	Get(ctx context.Context) Execer
}

type ConnGetter struct {
	db *sqlx.DB
}

func NewConnGetter(db *sqlx.DB) *ConnGetter {
	return &ConnGetter{db: db}
}

func (g *ConnGetter) Get(ctx context.Context) Execer {
	return g.db
}
