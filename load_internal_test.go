// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"fmt"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

// loadQ is a Querier that only provides a dialect; compiling statements never
// touches the connection.
type loadQ struct{ d *dialect.Dialect }

func (q loadQ) Dialect() *dialect.Dialect { return q.d }
func (q loadQ) conn() *execer             { return &execer{d: q.d} }

// kid is a child row: ID is its key to the parent, Seq distinguishes siblings.
type kid struct{ Pid, Seq int }

// par is a parent row; Pid < 0 means NULL key.
type par struct {
	Pid  int
	Kids []kid
	One  *kid
}

var kidMeta = Meta[kid]{
	Table:   "kids",
	Columns: []ColumnMeta{{Name: "pid"}, {Name: "seq"}},
	Ptr: func(m *kid, i int) any {
		if i == 0 {
			return &m.Pid
		}
		return &m.Seq
	},
}

func kidsRel(kind RelationKind) Relation[par, kid] {
	r := Relation[par, kid]{
		Name: "Kids", Kind: kind,
		Parent: TableRef{Name: "pars"}, Child: TableRef{Name: "kids"},
		ChildMeta:  &kidMeta,
		ParentCols: []string{"pid"}, ChildCols: []string{"pid"}, ChildPK: []string{"seq"},
		ParentKey: func(p *par) ([]any, bool) { return []any{p.Pid}, p.Pid >= 0 },
		ChildKey:  func(c *kid) ([]any, bool) { return []any{c.Pid}, true },
		GetMany:   func(p *par) []kid { return p.Kids },
		SetMany:   func(p *par, cs []kid) { p.Kids = cs },
		GetOne:    func(p *par) *kid { return p.One },
		SetOne:    func(p *par, c *kid) { p.One = c },
	}
	return r
}

// sliceRows serves kids as a result set.
type sliceRows struct {
	rows []kid
	i    int
	cur  kid
}

func (r *sliceRows) Next() bool {
	if r.i >= len(r.rows) {
		return false
	}
	r.cur = r.rows[r.i]
	r.i++
	return true
}

func (r *sliceRows) Scan(dest ...any) error {
	*(dest[0].(*int)) = r.cur.Pid
	*(dest[1].(*int)) = r.cur.Seq
	return nil
}
func (r *sliceRows) Err() error   { return nil }
func (r *sliceRows) Close() error { return nil }

func pendingOf(t *testing.T, r Relation[par, kid], parents []par) *pending[par, kid] {
	t.Helper()
	pl, err := r.prepareWith(loadQ{dialect.For(dialect.Postgres)}, parents, &loadConfig{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return pl.(*pending[par, kid])
}

func TestLoadGroupsDedupeAndSkipNilKeys(t *testing.T) {
	parents := []par{{Pid: 7}, {Pid: -1}, {Pid: 3}, {Pid: 7}, {Pid: 9}}
	pd := pendingOf(t, kidsRel(HasMany), parents)
	if got := fmt.Sprint(pd.keys); got != "[[7] [3] [9]]" {
		t.Fatalf("distinct non-nil keys in first-seen order: %s", got)
	}
	if len(pd.queries) != 1 {
		t.Fatalf("want one statement, got %d", len(pd.queries))
	}
	if got := fmt.Sprint(pd.queries[0].Args); got != "[7 3 9]" {
		t.Fatalf("nil key must not be queried and duplicates collapse: %s", got)
	}

	none := pendingOf(t, kidsRel(HasMany), []par{{Pid: -1}, {Pid: -2}})
	if len(none.queries) != 0 {
		t.Fatalf("all-NULL parents must issue no statement, got %d", len(none.queries))
	}
}

func TestLoadAssignsInPoolOrderToEveryDuplicateParent(t *testing.T) {
	parents := []par{{Pid: 7}, {Pid: -1}, {Pid: 3}, {Pid: 7}, {Pid: 5}}
	pd := pendingOf(t, kidsRel(HasMany), parents)
	rows := &sliceRows{rows: []kid{{3, 1}, {7, 1}, {7, 2}, {7, 3}, {42, 1}}}
	if err := pd.consume(rows); err != nil {
		t.Fatal(err)
	}
	if err := pd.finish(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(parents[0].Kids); got != "[{7 1} {7 2} {7 3}]" {
		t.Fatalf("parent 0: %s", got)
	}
	if got := fmt.Sprint(parents[3].Kids); got != "[{7 1} {7 2} {7 3}]" {
		t.Fatalf("duplicate parent must get the same children: %s", got)
	}
	if got := fmt.Sprint(parents[2].Kids); got != "[{3 1}]" {
		t.Fatalf("parent 2: %s", got)
	}
	if parents[1].Kids != nil || parents[4].Kids != nil {
		t.Fatalf("NULL-key and unmatched parents stay empty: %v %v", parents[1].Kids, parents[4].Kids)
	}
}

func TestLoadClearsStaleAssociation(t *testing.T) {
	parents := []par{{Pid: 1, Kids: []kid{{9, 9}}, One: &kid{9, 9}}}
	pd := pendingOf(t, kidsRel(HasMany), parents)
	if err := pd.finish(context.Background()); err != nil {
		t.Fatal(err)
	}
	if parents[0].Kids != nil {
		t.Fatalf("stale children must be cleared: %v", parents[0].Kids)
	}
	pd = pendingOf(t, kidsRel(HasOne), parents)
	if err := pd.finish(context.Background()); err != nil {
		t.Fatal(err)
	}
	if parents[0].One != nil {
		t.Fatal("stale to-one must be cleared")
	}
}

func TestLoadHasOneRejectsTwoChildren(t *testing.T) {
	parents := []par{{Pid: 1}}
	pd := pendingOf(t, kidsRel(HasOne), parents)
	if err := pd.consume(&sliceRows{rows: []kid{{1, 1}, {1, 2}}}); err != nil {
		t.Fatal(err)
	}
	if err := pd.finish(context.Background()); err == nil {
		t.Fatal("has-one matching two children must fail")
	}
}

func TestLoadChunksUnderBindLimitPreservingOrder(t *testing.T) {
	d := *dialect.For(dialect.Postgres)
	d.MaxParams = 40 // 16 reserved -> 24 keys per statement
	parents := make([]par, 60)
	for i := range parents {
		parents[i].Pid = i + 1
	}
	rel := kidsRel(HasMany)
	pl, err := rel.prepareWith(loadQ{&d}, parents, &loadConfig{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	pd := pl.(*pending[par, kid])
	if len(pd.queries) != 3 {
		t.Fatalf("60 keys at 24 per statement: want 3 statements, got %d", len(pd.queries))
	}
	next := 1
	for i, q := range pd.queries {
		if len(q.Args) > d.MaxParams {
			t.Fatalf("statement %d binds %d > limit %d", i, len(q.Args), d.MaxParams)
		}
		for _, a := range q.Args {
			if a != next {
				t.Fatalf("statement %d: key %v out of order, want %d", i, a, next)
			}
			next++
		}
	}
	if next != 61 {
		t.Fatalf("keys lost across chunks: next=%d", next)
	}
}

func TestLoadReservesScopeArgs(t *testing.T) {
	d := *dialect.For(dialect.Postgres)
	d.MaxParams = 40
	rel := kidsRel(HasMany)
	rel.Discriminator = &Discriminator{Column: "kind", Value: "x"}
	parents := make([]par, 30)
	for i := range parents {
		parents[i].Pid = i + 1
	}
	pl, err := rel.prepareWith(loadQ{&d}, parents, &loadConfig{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i, q := range pl.(*pending[par, kid]).queries {
		if len(q.Args) > d.MaxParams {
			t.Fatalf("statement %d binds %d > limit %d", i, len(q.Args), d.MaxParams)
		}
	}
}

func TestKeysPerStatementCompositeCap(t *testing.T) {
	pg := dialect.For(dialect.Postgres)
	if got := keysPerStatement(pg, 1, 0); got != 65535 {
		t.Fatalf("single-column keys use the full bind budget, got %d", got)
	}
	if got := keysPerStatement(pg, 2, 0); got != maxCompositeTuples {
		t.Fatalf("composite keys are capped at %d tuples, got %d", maxCompositeTuples, got)
	}
	tiny := *pg
	tiny.MaxParams = 10
	if got := keysPerStatement(&tiny, 2, 0); got != 5 {
		t.Fatalf("a tight bind limit still wins over the cap, got %d", got)
	}
}
