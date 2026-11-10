// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// rqRow is a projection of the rq_items table, in a column order the
// statements below deliberately do not follow.
type rqRow struct {
	momo.Projection
	ID   int64
	Name string
}

var rqRowMeta = momo.Meta[rqRow]{
	Columns:  []momo.ColumnMeta{{Name: "id"}, {Name: "name"}},
	Ptr:      func(m *rqRow, i int) any { return []any{&m.ID, &m.Name}[i] },
	Value:    func(m *rqRow, i int) any { return []any{m.ID, m.Name}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*rqRow) MomoMeta() *momo.Meta[rqRow] { return &rqRowMeta }

// rqEach runs fn on PostgreSQL and SQLite (testopen.Each) and on each MySQL
// flavor; a dialect the job did not select is skipped.
func rqEach(t *testing.T, fn func(t *testing.T, db *momo.DB, d dialect.Name)) {
	t.Helper()
	testopen.Each(t, fn)
	for name, open := range map[string]func(*testing.T, ...momo.Option) *momo.DB{
		"mysql84": testopen.MySQL84, "mysql97": testopen.MySQL97, "mariadb": testopen.MariaDB,
	} {
		t.Run(name, func(t *testing.T) { fn(t, open(t), dialect.MySQL) })
	}
}

func rqSetup(t *testing.T, db *momo.DB) {
	t.Helper()
	ctx := context.Background()
	for _, s := range []string{
		"CREATE TABLE rq_items (id INTEGER PRIMARY KEY, name VARCHAR(40) NOT NULL UNIQUE)",
		"INSERT INTO rq_items (id, name) VALUES (1, 'one'), (2, 'two'), (3, 'three')",
	} {
		if _, err := momo.Exec(ctx, db, s); err != nil {
			t.Fatal(err)
		}
	}
}

// Catches: multi-column raw results mis-scanned on a real driver (tuple and
// name-matched projection), on a *DB, a *Tx and a pinned handle.
func TestQueryMultiColumn(t *testing.T) {
	rqEach(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		rqSetup(t, db)
		ctx := context.Background()
		const stmt = "SELECT name, id FROM rq_items WHERE id > ? ORDER BY id"

		check := func(t *testing.T, q momo.Querier) {
			t.Helper()
			tup, err := momo.Query[momo.Row2[string, int64]](ctx, q, stmt, 1)
			if err != nil || len(tup) != 2 || tup[0].C1 != "two" || tup[1].C2 != 3 {
				t.Fatalf("tuple = %+v, %v", tup, err)
			}
			proj, err := momo.Query[rqRow](ctx, q, stmt, 1)
			if err != nil || len(proj) != 2 || proj[0].ID != 2 || proj[0].Name != "two" {
				t.Fatalf("projection (columns reordered) = %+v, %v", proj, err)
			}
		}
		check(t, db)
		if err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			check(t, tx)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if pinner, ok := db.Driver().(momo.Pinner); ok {
			pc, err := pinner.Pin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = pc.Release() }()
			check(t, db.OnConn(pc))
		}

		if d == dialect.Postgres {
			got, err := momo.Query[momo.Row2[int64, string]](ctx, db,
				"INSERT INTO rq_items (id, name) VALUES ($1, $2) RETURNING id, name", 10, "ten")
			if err != nil || len(got) != 1 || got[0].C1 != 10 || got[0].C2 != "ten" {
				t.Fatalf("RETURNING = %+v, %v", got, err)
			}
		}
	})
}

// Catches: an unknown, missing or surplus column going unreported on a real
// driver.
func TestQueryColumnErrors(t *testing.T) {
	rqEach(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		rqSetup(t, db)
		ctx := context.Background()
		for name, stmt := range map[string]string{
			"unknown": "SELECT id, name, id AS extra FROM rq_items",
			"missing": "SELECT id FROM rq_items",
		} {
			if _, err := momo.Query[rqRow](ctx, db, stmt); !errors.Is(err, momo.ErrValidation) {
				t.Errorf("%s column: err = %v, want ErrValidation", name, err)
			}
		}
		if _, err := momo.Query[momo.Row1[int64]](ctx, db, "SELECT id, name FROM rq_items"); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("tuple arity: err = %v, want ErrValidation", err)
		}
	})
}

// Catches: the streaming form buffering, or leaking its cursor on an early
// break (the next statement would then block on a one-connection pool).
func TestQueryRowsStreaming(t *testing.T) {
	rqEach(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		rqSetup(t, db)
		ctx := context.Background()
		n := 0
		for r, err := range momo.QueryRows[rqRow](ctx, db, "SELECT name, id FROM rq_items ORDER BY id") {
			if err != nil {
				t.Fatal(err)
			}
			n++
			if r.ID == 2 {
				break
			}
		}
		if n != 2 {
			t.Fatalf("read %d rows before the break, want 2", n)
		}
		var firstErr error
		for _, err := range momo.QueryRows[momo.Row1[int64]](ctx, db, "SELECT nope FROM rq_items") {
			firstErr = err
			break
		}
		if firstErr == nil {
			t.Fatal("a failing statement must yield its error")
		}
		if c, err := momo.Query[momo.Row1[int64]](ctx, db, "SELECT count(*) FROM rq_items"); err != nil || c[0].C1 != 3 {
			t.Fatalf("handle unusable after the streams: %v, %v", c, err)
		}
	})
}

// Catches: Query classifying errors differently from Exec, or exposing the
// bound value in a message Exec keeps clean.
func TestQueryErrorsClassifiedLikeExec(t *testing.T) {
	rqEach(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		rqSetup(t, db)
		ctx := context.Background()
		const secret = "s3cret-value-xyz"
		if _, err := momo.Exec(ctx, db, "INSERT INTO rq_items (id, name) VALUES (?, ?)", 20, secret); err != nil {
			t.Fatal(err)
		}
		_, execErr := momo.Exec(ctx, db, "INSERT INTO rq_items (id, name) VALUES (?, ?)", 21, secret)
		_, queryErr := momo.Query[momo.Row1[int64]](ctx, db, "SELECT id FROM rq_items WHERE name = ?", secret)
		if queryErr != nil {
			t.Fatal(queryErr)
		}
		if !errors.Is(execErr, momo.ErrUniqueViolation) {
			t.Fatalf("Exec duplicate = %v, want ErrUniqueViolation", execErr)
		}
		qErr := func() error {
			// A data-modifying statement through Query; PostgreSQL and SQLite
			// accept RETURNING, MySQL has none, so use a failing SELECT there.
			if d == dialect.MySQL {
				_, err := momo.Query[momo.Row1[int64]](ctx, db, "SELECT * FROM rq_missing_table")
				return err
			}
			_, err := momo.Query[momo.Row1[int64]](ctx, db, "INSERT INTO rq_items (id, name) VALUES (?, ?) RETURNING id", 22, secret)
			return err
		}()
		if qErr == nil {
			t.Fatal("expected an error")
		}
		if d != dialect.MySQL && !errors.Is(qErr, momo.ErrUniqueViolation) {
			t.Fatalf("Query duplicate = %v, want ErrUniqueViolation", qErr)
		}
		if strings.Contains(qErr.Error(), secret) != strings.Contains(execErr.Error(), secret) {
			t.Fatalf("Query error %q and Exec error %q disagree on exposing the bound value", qErr, execErr)
		}

		cctx, cancel := context.WithCancel(ctx)
		cancel()
		_, execErr = momo.Exec(cctx, db, "UPDATE rq_items SET name = name")
		_, queryErr = momo.Query[momo.Row1[int64]](cctx, db, "SELECT id FROM rq_items")
		if execErr == nil || queryErr == nil {
			t.Fatalf("a canceled context must fail: exec %v, query %v", execErr, queryErr)
		}
		if errors.Is(execErr, momo.ErrCanceled) != errors.Is(queryErr, momo.ErrCanceled) {
			t.Fatalf("cancellation classified differently: exec %v, query %v", execErr, queryErr)
		}
	})
}
