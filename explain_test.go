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
	"github.com/azrtydxb/momo/vecop"
	"github.com/azrtydxb/momo/vector"
)

func TestExplainStatementPerDialect(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{"Seq Scan on docs"}, {"  Filter: x"}})
	plan, err := momo.Select[vecDocU, *vecDocU](momo.New(drv)).Explain(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if plan != "Seq Scan on docs\n  Filter: x" {
		t.Fatalf("plan = %q, want rows joined by newlines", plan)
	}
	calls := drv.Calls()
	if got := calls[len(calls)-1].SQL; !strings.HasPrefix(got, "EXPLAIN SELECT") || strings.Contains(got, "ANALYZE") {
		t.Fatalf("SQL = %q, want a plain EXPLAIN (never ANALYZE)", got)
	}

	// SQLite: EXPLAIN QUERY PLAN returns (id, parent, notused, detail); the
	// detail column is the plan line.
	sd := fakedriver.New(dialect.SQLite)
	sd.SetRows([][]any{{int64(2), int64(0), int64(0), "SCAN docs"}})
	plan, err = momo.Select[vecDocU, *vecDocU](momo.New(sd)).Explain(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if plan != "SCAN docs" {
		t.Fatalf("sqlite plan = %q", plan)
	}
	calls = sd.Calls()
	if got := calls[len(calls)-1].SQL; !strings.HasPrefix(got, "EXPLAIN QUERY PLAN SELECT") {
		t.Fatalf("sqlite SQL = %q", got)
	}

	// MySQL: tabular EXPLAIN is not parsed; the call is refused before any SQL.
	md := fakedriver.New(dialect.MySQL)
	_, err = momo.Select[vecDocU, *vecDocU](momo.New(md)).Explain(t.Context())
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("mysql err = %v, want ErrUnsupported", err)
	}
	if len(md.Calls()) != 0 {
		t.Fatal("mysql ran a statement")
	}
}

func TestExplainBindsArgsNeverInterpolates(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{"plan"}})
	db := momo.New(drv)
	q := momo.Select[vecDocU, *vecDocU](db).Where(momo.NewColumn[int64]("docs", "id", 0).Eq(int64(41))).
		OrderBy(vecop.L2(momo.NewVectorColumn[vector.Vector]("docs", "emb", 3, 0), vector.Vector{1, 2, 3}).Asc())
	c, err := q.SQL()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Explain(t.Context()); err != nil {
		t.Fatal(err)
	}
	calls := drv.Calls()
	got := calls[len(calls)-1]
	if got.SQL != "EXPLAIN "+c.SQL {
		t.Fatalf("SQL = %q, want EXPLAIN + %q", got.SQL, c.SQL)
	}
	if !reflect.DeepEqual(got.Args, c.Args) || len(got.Args) == 0 {
		t.Fatalf("args = %v, want the query's own args %v", got.Args, c.Args)
	}
	if strings.Contains(got.SQL, "41") {
		t.Fatalf("value interpolated into SQL: %q", got.SQL)
	}
}

func TestExplainBuildErrorRunsNothing(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv)
	n := len(drv.Calls())
	col := momo.NewVectorColumn[vector.Vector]("docs", "emb", 3, 0)
	_, err := momo.Select[vecDocU, *vecDocU](db).
		OrderBy(vecop.L2(col, vector.Vector{1, 2}).Asc()).
		Explain(t.Context())
	if !errors.Is(err, momo.ErrInvalidVector) {
		t.Fatalf("err = %v, want ErrInvalidVector from the build", err)
	}
	if len(drv.Calls()) != n {
		t.Fatal("a statement ran despite the build error")
	}
}

type vecDocU struct {
	ID  int64
	Emb vector.Vector
}

var vecDocUMeta = momo.Meta[vecDocU]{
	Table:      "docs",
	Columns:    []momo.ColumnMeta{{Name: "id"}, {Name: "emb"}},
	Ptr:        func(m *vecDocU, i int) any { return []any{&m.ID, &m.Emb}[i] },
	Value:      func(m *vecDocU, i int) any { return []any{m.ID, m.Emb}[i] },
	Identity:   -1,
	Version:    -1,
	SoftDelete: -1,
	CreatedAt:  -1,
	UpdatedAt:  -1,
	PK:         []int{0},
}

func (d *vecDocU) MomoMeta() *momo.Meta[vecDocU] { return &vecDocUMeta }
