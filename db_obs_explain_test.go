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
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/internal/relfixture/author"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// obsAuthorCount counts the authors with the given id.
func obsAuthorCount(t testing.TB, q momo.Querier, id int64) int64 {
	t.Helper()
	n, err := momo.Select[models.Author](q).Where(author.ID.Eq(id)).Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// TestExplainAnalyzeExplicit: the ordinary paths never issue EXPLAIN, Explain
// of a write plans it without running it, and ExplainAnalyze runs the
// statement only on PostgreSQL and rolls a write back.
func TestExplainAnalyzeExplicit(t *testing.T) {
	ctx := context.Background()
	rec := &obstest.Recorder{}
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		obsSeed(t, db)
		rec.Reset()

		// Select, Count and Exec paths issue no EXPLAIN text.
		sel := momo.Select[models.Author](db).Where(author.ID.Eq(1))
		if _, err := sel.All(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := sel.Count(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := momo.Exec(ctx, db, `UPDATE authors SET name = ? WHERE id = ?`, "Ada", int64(1)); err != nil {
			t.Fatal(err)
		}
		for _, s := range rec.SQLs() {
			if strings.Contains(strings.ToUpper(s), "EXPLAIN") {
				t.Fatalf("ordinary path issued %q", s)
			}
		}

		// Explain of a write issues one EXPLAIN statement and does not run it.
		ins, err := momo.Insert[models.Author](db).Model(&models.Author{ID: 99, Name: "ghost"}).SQL()
		if err != nil {
			t.Fatal(err)
		}
		prefix := "EXPLAIN "
		if d == dialect.SQLite {
			prefix = "EXPLAIN QUERY PLAN "
		}
		start := len(rec.SQLs())
		if _, err := momo.Explain(ctx, db, ins); err != nil {
			t.Fatal(err)
		}
		stmts := rec.SQLs()[start:]
		if len(stmts) != 1 || stmts[0] != prefix+ins.SQL {
			t.Fatalf("Explain issued %q, want exactly %q", stmts, prefix+ins.SQL)
		}
		if n := obsAuthorCount(t, db, 99); n != 0 {
			t.Fatalf("Explain executed the INSERT: %d rows", n)
		}

		// Explain of a read returns plan lines that mention the table.
		c, err := sel.SQL()
		if err != nil {
			t.Fatal(err)
		}
		lines, err := momo.Explain(ctx, db, c)
		if err != nil {
			t.Fatal(err)
		}
		if len(lines) == 0 || !strings.Contains(strings.Join(lines, "\n"), "authors") {
			t.Fatalf("plan lines %q do not mention the table", lines)
		}

		// ExplainAnalyze.
		start = len(rec.SQLs())
		if d != dialect.Postgres {
			if _, err := momo.ExplainAnalyze(ctx, db, c); !errors.Is(err, momo.ErrUnsupported) {
				t.Fatalf("ExplainAnalyze on %s: err = %v, want ErrUnsupported", d, err)
			}
			if got := len(rec.SQLs()) - start; got != 0 {
				t.Fatalf("unsupported ExplainAnalyze issued %d statements", got)
			}
			return
		}
		lines, err = momo.ExplainAnalyze(ctx, db, ins)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(strings.Join(lines, "\n"), "actual time") {
			t.Fatalf("ANALYZE plan has no timings: %q", lines)
		}
		if n := obsAuthorCount(t, db, 99); n != 0 {
			t.Fatalf("ExplainAnalyze left the INSERT committed: %d rows", n)
		}
		lines, err = momo.ExplainAnalyze(ctx, db, c)
		if err != nil {
			t.Fatal(err)
		}
		if len(lines) == 0 {
			t.Fatal("ExplainAnalyze of a SELECT returned no plan")
		}

		// A SELECT can write through a function; ANALYZE must not keep that.
		if _, err := momo.Exec(ctx, db, `CREATE FUNCTION obs_write() RETURNS int LANGUAGE sql AS $f$ INSERT INTO authors (id, name) VALUES (98, 'fn') RETURNING 1 $f$`); err != nil {
			t.Fatal(err)
		}
		if _, err := momo.ExplainAnalyze(ctx, db, momo.Compiled{SQL: `SELECT obs_write()`}); err != nil {
			t.Fatal(err)
		}
		if n := obsAuthorCount(t, db, 98); n != 0 {
			t.Fatalf("ExplainAnalyze of a SELECT kept the write of a function it called: %d rows", n)
		}

		// Inside a transaction the write is rolled back to a savepoint and the
		// transaction stays usable.
		err = db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			if _, err := momo.ExplainAnalyze(ctx, tx, ins); err != nil {
				return err
			}
			if n := obsAuthorCount(t, tx, 99); n != 0 {
				t.Errorf("ExplainAnalyze in a Tx left the INSERT in place: %d rows", n)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}, momo.WithHook(rec))
}
