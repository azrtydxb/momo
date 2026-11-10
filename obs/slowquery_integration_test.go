// SPDX-License-Identifier: Apache-2.0

//go:build integration

package obs_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/author"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/obs"
)

const slowCTE = `WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM c WHERE x < 2000000) SELECT count(*) FROM c`
const slowFailingCTE = `WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM c WHERE x < 2000000) SELECT sum(x) / (count(*) - count(*)) FROM c`
const slowCTEParam = `WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM c WHERE x < ?) SELECT count(*) FROM c`

// badExplainDriver turns every EXPLAIN statement into one that fails (a
// missing table), the way a rejected EXPLAIN would. On PostgreSQL such a
// failure aborts the transaction unless a savepoint contains it.
type badExplainDriver struct{ momo.Driver }

func failExplain(sql string, args []any) (string, []any) {
	if strings.HasPrefix(sql, "EXPLAIN ") {
		return `SELECT * FROM no_such_table_for_explain`, nil
	}
	return sql, args
}

func (d badExplainDriver) Query(ctx context.Context, sql string, args []any) (momo.Rows, error) {
	sql, args = failExplain(sql, args)
	return d.Driver.Query(ctx, sql, args)
}

func (d badExplainDriver) Exec(ctx context.Context, sql string, args []any) (int64, error) {
	sql, args = failExplain(sql, args)
	return d.Driver.Exec(ctx, sql, args)
}

func (d badExplainDriver) Begin(ctx context.Context, opts momo.TxOptions) (momo.DriverTx, error) {
	tx, err := d.Driver.Begin(ctx, opts)
	if err != nil {
		return nil, err
	}
	return badExplainTx{tx}, nil
}

type badExplainTx struct{ momo.DriverTx }

func (t badExplainTx) Query(ctx context.Context, sql string, args []any) (momo.Rows, error) {
	sql, args = failExplain(sql, args)
	return t.DriverTx.Query(ctx, sql, args)
}

func (t badExplainTx) Exec(ctx context.Context, sql string, args []any) (int64, error) {
	sql, args = failExplain(sql, args)
	return t.DriverTx.Exec(ctx, sql, args)
}

type slowSink struct {
	mu sync.Mutex
	qs []obs.SlowQuery
}

func (s *slowSink) add(q obs.SlowQuery) {
	s.mu.Lock()
	s.qs = append(s.qs, q)
	s.mu.Unlock()
}

// match returns the reports whose SQL satisfies keep, ignoring unrelated
// statements (connection set-up, BEGIN) that may also cross a low threshold.
func (s *slowSink) match(keep func(sql string) bool) []obs.SlowQuery {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []obs.SlowQuery
	for _, q := range s.qs {
		if keep(q.SQL) {
			out = append(out, q)
		}
	}
	return out
}

// only returns the reports for exactly sql.
func (s *slowSink) only(sql string) []obs.SlowQuery {
	return s.match(func(got string) bool { return got == sql })
}

// param returns the reports of slowCTEParam, whose placeholder the dialect
// may have renumbered.
func (s *slowSink) param() []obs.SlowQuery {
	return s.match(func(got string) bool {
		return strings.HasPrefix(got, "WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM c WHERE x < ") && !strings.Contains(got, "2000000")
	})
}

func (s *slowSink) anyContaining(sub string) bool {
	return len(s.match(func(got string) bool { return strings.Contains(got, sub) })) > 0
}

func authorRows(t testing.TB, q momo.Querier, id int64) int64 {
	t.Helper()
	n, err := momo.Select[models.Author](q).Where(author.ID.Eq(id)).Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSlowQueryHook(t *testing.T) {
	ctx := context.Background()
	const threshold = 50 * time.Millisecond
	rec := &obs.Recorder{}
	// plain carries only the recorder: it is the hook-free querier whose
	// EXPLAIN statements the recorder lists.
	testopen.Each(t, func(t *testing.T, plain *momo.DB, d dialect.Name) {
		relfixture.CreateSchema(t, plain)
		relfixture.SeedBasic(t, plain)
		if _, err := momo.Exec(ctx, plain, `SELECT 1`); err != nil { // warm the pool
			t.Fatal(err)
		}
		drv := plain.Driver()

		t.Run("reports only slow statements and captures a plain plan", func(t *testing.T) {
			rec.Reset()
			sink := &slowSink{}
			db := momo.New(drv, momo.WithHook(obs.NewSlowQuery(threshold, sink.add, obs.WithPlainExplain(plain))))
			if _, err := momo.Select[models.Author](db).Where(author.ID.Eq(1)).One(ctx); err != nil {
				t.Fatal(err)
			}
			if sink.anyContaining("authors") {
				t.Fatal("a fast statement was reported")
			}
			if _, err := momo.Exec(ctx, db, slowCTE); err != nil {
				t.Fatal(err)
			}
			got := sink.only(slowCTE)
			if len(got) != 1 {
				t.Fatalf("want 1 slow report, got %d", len(got))
			}
			if got[0].Duration <= threshold {
				t.Fatalf("report %+v", got[0])
			}
			if len(got[0].Plan) == 0 || got[0].PlanSkipped != "" {
				t.Fatalf("Plan = %q PlanSkipped = %q, want a plan", got[0].Plan, got[0].PlanSkipped)
			}
			explains := 0
			for _, s := range rec.SQLs() {
				if strings.Contains(strings.ToUpper(s), "ANALYZE") {
					t.Fatalf("ANALYZE was issued: %q", s)
				}
				if strings.HasPrefix(s, "EXPLAIN ") {
					explains++
				}
			}
			if explains != 1 {
				t.Fatalf("want exactly one EXPLAIN statement, got %d", explains)
			}
		})

		t.Run("no plan unless WithPlainExplain is given", func(t *testing.T) {
			sink := &slowSink{}
			db := momo.New(drv, momo.WithHook(obs.NewSlowQuery(threshold, sink.add)))
			if _, err := momo.Exec(ctx, db, slowCTE); err != nil {
				t.Fatal(err)
			}
			got := sink.only(slowCTE)
			if len(got) != 1 || got[0].Plan != nil || got[0].PlanSkipped != "" {
				t.Fatalf("report %+v, want a bare report", got)
			}
		})

		t.Run("plan captured inside a transaction through a savepoint", func(t *testing.T) {
			sink := &slowSink{}
			db := momo.New(drv, momo.WithHook(obs.NewSlowQuery(threshold, sink.add, obs.WithPlainExplain(plain))))
			err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
				if _, err := momo.Exec(ctx, tx, slowCTE); err != nil {
					return err
				}
				_, err := momo.Exec(ctx, tx, `SELECT 1`)
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			got := sink.only(slowCTE)
			if len(got) != 1 || len(got[0].Plan) == 0 || got[0].PlanSkipped != "" {
				t.Fatalf("report %+v, want a plan captured in the transaction", got)
			}
		})

		t.Run("a failing capture leaves the transaction usable", func(t *testing.T) {
			sink := &slowSink{}
			db := momo.New(badExplainDriver{drv}, momo.WithHook(obs.NewSlowQuery(threshold, sink.add, obs.WithPlainExplain(plain))))
			err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
				if _, err := momo.Exec(ctx, tx, slowCTE); err != nil {
					return err
				}
				_, err := momo.Exec(ctx, tx, `INSERT INTO authors (id, name) VALUES (?, ?)`, int64(77), "after")
				return err
			})
			if err != nil {
				t.Fatalf("transaction broken by the failed capture: %v", err)
			}
			got := sink.only(slowCTE)
			if len(got) != 1 || got[0].Plan != nil || !strings.Contains(got[0].PlanSkipped, "EXPLAIN failed") {
				t.Fatalf("report %+v, want PlanSkipped to say EXPLAIN failed", got)
			}
			if n := authorRows(t, plain, 77); n != 1 {
				t.Fatalf("row written after the failed capture is missing: %d", n)
			}
		})

		t.Run("an aborted transaction is reported as such", func(t *testing.T) {
			if d != dialect.Postgres {
				t.Skip("only PostgreSQL aborts a transaction after a failed statement")
			}
			sink := &slowSink{}
			db := momo.New(drv, momo.WithHook(obs.NewSlowQuery(threshold, sink.add, obs.WithPlainExplain(plain))))
			err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
				_, err := momo.Exec(ctx, tx, slowFailingCTE)
				return err
			})
			if err == nil {
				t.Fatal("want the division by zero error")
			}
			got := sink.only(slowFailingCTE)
			if len(got) != 1 || got[0].Plan != nil || got[0].PlanSkipped != "plan unavailable: transaction aborted" {
				t.Fatalf("report %+v, want 'plan unavailable: transaction aborted'", got)
			}
		})

		t.Run("parameterized statements need WithArgLogging", func(t *testing.T) {
			quiet, loud := &slowSink{}, &slowSink{}
			dbQuiet := momo.New(drv, momo.WithHook(obs.NewSlowQuery(threshold, quiet.add, obs.WithPlainExplain(plain))))
			dbLoud := momo.New(drv, momo.WithArgLogging(), momo.WithHook(obs.NewSlowQuery(threshold, loud.add, obs.WithPlainExplain(plain))))
			for _, db := range []*momo.DB{dbQuiet, dbLoud} {
				if _, err := momo.Exec(ctx, db, slowCTEParam, 2000000); err != nil {
					t.Fatal(err)
				}
			}
			q, l := quiet.param(), loud.param()
			if len(q) != 1 || q[0].Plan != nil || !strings.Contains(q[0].PlanSkipped, "arguments") {
				t.Fatalf("without WithArgLogging: %+v", q)
			}
			if len(l) != 1 || len(l[0].Plan) == 0 {
				t.Fatalf("with WithArgLogging: %+v", l)
			}
		})

		t.Run("the explain statement is not reported again", func(t *testing.T) {
			first, second := &slowSink{}, &slowSink{}
			// observed has its own slow hook with a 1ns threshold: without the
			// recursion guard it would report the EXPLAIN statement.
			observed := momo.New(drv, momo.WithHook(obs.NewSlowQuery(time.Nanosecond, second.add)))
			db := momo.New(drv, momo.WithHook(obs.NewSlowQuery(threshold, first.add, obs.WithPlainExplain(observed))))
			if _, err := momo.Exec(ctx, db, slowCTE); err != nil {
				t.Fatal(err)
			}
			if second.anyContaining("EXPLAIN") {
				t.Fatal("an EXPLAIN statement was reported")
			}
			if len(first.only(slowCTE)) != 1 {
				t.Fatalf("want 1 report, got %d", len(first.only(slowCTE)))
			}
		})
	}, momo.WithHook(rec))
}
