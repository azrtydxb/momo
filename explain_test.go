// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

func TestExplainStatementPerDialectFree(t *testing.T) {
	c := momo.Compiled{SQL: "INSERT INTO authors (id, name) VALUES ($1, $2)", Args: []any{int64(9), "x"}}
	for _, tc := range []struct {
		d      dialect.Name
		prefix string
		rows   [][]any
		want   []string
	}{
		{dialect.Postgres, "EXPLAIN ", [][]any{{"Insert on authors"}, {"  ->  Result"}}, []string{"Insert on authors", "  ->  Result"}},
		{dialect.SQLite, "EXPLAIN QUERY PLAN ", [][]any{{int64(2), int64(0), int64(0), "SCAN authors"}}, []string{"SCAN authors"}},
		{dialect.MySQL, "EXPLAIN FORMAT=JSON ", [][]any{{`{"query_block": {}}`}}, []string{`{"query_block": {}}`}},
	} {
		drv := fakedriver.New(tc.d)
		drv.SetRows(tc.rows)
		got, err := momo.Explain(t.Context(), momo.New(drv), c)
		if err != nil {
			t.Fatalf("%s: %v", tc.d, err)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: plan = %q, want %q", tc.d, got, tc.want)
		}
		calls := drv.Calls()
		if len(calls) != 1 || calls[0].SQL != tc.prefix+c.SQL || !reflect.DeepEqual(calls[0].Args, c.Args) {
			t.Errorf("%s: calls = %+v, want one %q statement with the arguments bound", tc.d, calls, tc.prefix+c.SQL)
		}
	}
}

func TestExplainAnalyzeUnsupportedIssuesNothing(t *testing.T) {
	for _, d := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		drv := fakedriver.New(d)
		_, err := momo.ExplainAnalyze(t.Context(), momo.New(drv), momo.Compiled{SQL: "SELECT 1"})
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s: err = %v, want ErrUnsupported", d, err)
		}
		if n := len(drv.Calls()); n != 0 {
			t.Errorf("%s: unsupported ExplainAnalyze issued %d statements", d, n)
		}
	}
}

func TestExplainAnalyzeReadRunsAsIs(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{"Seq Scan on authors (actual time=0.01..0.02 rows=1 loops=1)"}})
	lines, err := momo.ExplainAnalyze(t.Context(), momo.New(drv), momo.Compiled{SQL: "SELECT id FROM authors WHERE id = $1", Args: []any{int64(1)}})
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || !strings.Contains(lines[0], "actual time") {
		t.Fatalf("lines = %q", lines)
	}
	calls := drv.Calls()
	if len(calls) != 1 || !strings.HasPrefix(calls[0].SQL, "EXPLAIN (ANALYZE, FORMAT TEXT) SELECT") {
		t.Fatalf("calls = %+v, want one EXPLAIN ANALYZE of the SELECT", calls)
	}
	if drv.Commits() != 0 || drv.Rollbacks() != 0 {
		t.Fatalf("a read opened a transaction: commits=%d rollbacks=%d", drv.Commits(), drv.Rollbacks())
	}
}

// A write is analyzed inside a transaction that is rolled back, never committed.
func TestExplainAnalyzeWriteRollsBack(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{"Insert on authors (actual time=0.01..0.01 rows=0 loops=1)"}})
	lines, err := momo.ExplainAnalyze(t.Context(), momo.New(drv), momo.Compiled{SQL: "INSERT INTO authors (id, name) VALUES ($1, $2)", Args: []any{int64(9), "x"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 {
		t.Fatalf("lines = %q", lines)
	}
	if drv.Commits() != 0 || drv.Rollbacks() != 1 {
		t.Fatalf("commits=%d rollbacks=%d, want the write rolled back", drv.Commits(), drv.Rollbacks())
	}
}
