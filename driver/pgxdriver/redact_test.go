// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

const encodeSecret = "s3cr3t-bound-value"

var errEncodeCause = errors.New("encode cause")

// pgxEncodeError mimics the text pgx gives a client-side encode failure: it
// quotes the bound value.
func pgxEncodeError() error {
	inner := fmt.Errorf("unable to encode %#v into text format for int4 (OID 23): %w", encodeSecret, errEncodeCause)
	return fmt.Errorf("failed to encode args[2]: %w", inner)
}

func TestClassifyErrorRedactsEncodeErrors(t *testing.T) {
	got := ClassifyError(pgxEncodeError())
	for name, s := range map[string]string{"Error": got.Error(), "%v": fmt.Sprintf("%v", got), "%+v": fmt.Sprintf("%+v", got)} {
		if strings.Contains(s, encodeSecret) {
			t.Errorf("%s leaks the bound value: %q", name, s)
		}
	}
	if !strings.Contains(got.Error(), "args[2]") {
		t.Errorf("argument position lost: %q", got.Error())
	}
	if !errors.Is(got, errEncodeCause) {
		t.Error("cause no longer reachable with errors.Is")
	}
	// Redaction is idempotent.
	if again := ClassifyError(got); again.Error() != got.Error() {
		t.Errorf("second pass changed the message: %q", again.Error())
	}
	// Other client errors keep their text.
	plain := errors.New("connection refused")
	if ClassifyError(plain) != plain {
		t.Error("unrelated error was rewritten")
	}
}

// failingRows is a pgx.Rows whose Err reports a fixed error.
type failingRows struct {
	pgx.Rows
	err error
}

func (r failingRows) Err() error { return r.err }
func (r failingRows) Close()     {}

func TestRowsAndTxErrorsAreRedacted(t *testing.T) {
	r := pgxRows{failingRows{err: pgxEncodeError()}}
	for name, err := range map[string]error{
		"rows.Err":   r.Err(),
		"rows.Close": r.Close(),
		"txErr":      txErr("exec", pgxEncodeError()),
	} {
		if err == nil || strings.Contains(err.Error(), encodeSecret) {
			t.Errorf("%s = %v: want a redacted error", name, err)
		}
	}
}

func TestFromConnConfigRefusesSimpleProtocol(t *testing.T) {
	cfg, err := pgx.ParseConfig("postgres://u@127.0.0.1:1/db")
	if err != nil {
		t.Fatal(err)
	}
	cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	db, drv := FromConnConfig(cfg)
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if _, err := drv.Query(ctx, "SELECT $1", []any{1}); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("Query: %v, want ErrUnsupported", err)
	}
	if _, err := drv.Exec(ctx, "SELECT $1", []any{1}); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("Exec: %v, want ErrUnsupported", err)
	}
	if _, err := drv.Begin(ctx, momo.TxOptions{}); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("Begin: %v, want ErrUnsupported", err)
	}
	if _, err := drv.(momo.Pinner).Pin(ctx); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("Pin: %v, want ErrUnsupported", err)
	}
}

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
