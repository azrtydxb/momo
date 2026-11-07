// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/jackc/pgx/v5"
)

// stubPgxTx is a pgx.Tx that panics on any use; only identity matters here.
type stubPgxTx struct{ pgx.Tx }

func TestUnwrapMatchesDriverKindWithoutDatabase(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.Postgres))
	st := &sql.Tx{}
	pt := &stubPgxTx{}

	sqlAdopted := db.AdoptTx(SQLTx(st))
	if got, ok := UnwrapSQLTx(sqlAdopted); !ok || got != st {
		t.Fatalf("UnwrapSQLTx = %v, %v", got, ok)
	}
	if _, ok := UnwrapPgxTx(sqlAdopted); ok {
		t.Fatal("UnwrapPgxTx must be false for a sql.Tx")
	}

	pgxAdopted := db.AdoptTx(PgxTx(pt))
	if got, ok := UnwrapPgxTx(pgxAdopted); !ok || got != pgx.Tx(pt) {
		t.Fatalf("UnwrapPgxTx = %v, %v", got, ok)
	}
	if _, ok := UnwrapSQLTx(pgxAdopted); ok {
		t.Fatal("UnwrapSQLTx must be false for a pgx.Tx")
	}

	if _, ok := UnwrapSQLTx(nil); ok {
		t.Fatal("UnwrapSQLTx(nil) must be false")
	}
	if _, ok := UnwrapPgxTx(nil); ok {
		t.Fatal("UnwrapPgxTx(nil) must be false")
	}
	// A foreign driver's Tx is neither kind.
	fb, err := db.Begin(t.Context(), momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := UnwrapSQLTx(fb); ok {
		t.Fatal("fake driver tx is not a sql.Tx")
	}
	if _, ok := UnwrapPgxTx(fb); ok {
		t.Fatal("fake driver tx is not a pgx.Tx")
	}
}

func TestPgxTxRejectedExecModeReturnsErrUnsupported(t *testing.T) {
	ctx := t.Context()
	// stubPgxTx panics on any use, so the error must come before pgx is touched.
	dtx := PgxTx(&stubPgxTx{}, WithExecMode(pgx.QueryExecModeSimpleProtocol))
	if _, err := dtx.Query(ctx, "SELECT 1", nil); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("Query = %v, want ErrUnsupported", err)
	}
	if _, err := dtx.Exec(ctx, "SELECT 1", nil); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("Exec = %v, want ErrUnsupported", err)
	}
	if _, err := dtx.(momo.Batcher).SendBatch(ctx, []momo.Compiled{{SQL: "SELECT 1"}}); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("SendBatch = %v, want ErrUnsupported", err)
	}
	if _, err := dtx.(momo.Copier).CopyFrom(ctx, "t", []string{"a"}, [][]any{{1}}); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("CopyFrom = %v, want ErrUnsupported", err)
	}
}

func TestAdoptWrappersRejectNil(t *testing.T) {
	for name, f := range map[string]func(){
		"SQLTx": func() { SQLTx(nil) },
		"PgxTx": func() { PgxTx(nil) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s(nil) must panic", name)
				}
			}()
			f()
		}()
	}
}
