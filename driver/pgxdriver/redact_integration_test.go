// SPDX-License-Identifier: Apache-2.0

//go:build integration

package pgxdriver

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func TestFromSQLDBRefusesSimpleProtocolOnFirstUse(t *testing.T) {
	cfg, err := pgx.ParseConfig(testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	db := stdlib.OpenDB(*cfg)
	t.Cleanup(func() { _ = db.Close() })
	drv := FromSQLDB(db)
	if _, err := drv.Query(context.Background(), "SELECT $1::int", []any{1}); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("Query on a simple-protocol *sql.DB: %v, want ErrUnsupported", err)
	}
	if _, err := drv.Exec(context.Background(), "SELECT 1", nil); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("Exec stays refused: %v", err)
	}
}

func TestFromSQLDBAcceptsExtendedProtocol(t *testing.T) {
	db, err := sql.Open("pgx", testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	rows, err := FromSQLDB(db).Query(context.Background(), "SELECT $1::int", []any{1})
	if err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()
}

// encodeBomb cannot be encoded as int4; pgx quotes it in the error.
type encodeBomb struct{ S string }

func TestQueryAndExecEncodeErrorsAreRedacted(t *testing.T) {
	ctx := context.Background()
	drivers := map[string]momo.Driver{"pool": New(openPool(t))}
	db, err := sql.Open("pgx", testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	drivers["sqldb"] = FromSQLDB(db)
	for name, d := range drivers {
		t.Run(name, func(t *testing.T) {
			if _, err := d.Exec(ctx, "SELECT $1::int4", []any{encodeBomb{S: encodeSecret}}); err == nil || strings.Contains(err.Error(), encodeSecret) {
				t.Errorf("Exec error: %v", err)
			}
			rows, err := d.Query(ctx, "SELECT $1::int4", []any{encodeBomb{S: encodeSecret}})
			if err == nil {
				err = rows.Err()
				_ = rows.Close()
			}
			if err == nil || strings.Contains(err.Error(), encodeSecret) {
				t.Errorf("Query error: %v", err)
			}
		})
	}
}
