// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

// loadQ is a Querier that only provides a dialect; compiling statements never
// touches the connection.
type loadQ struct{ d *dialect.Dialect }

// classified makes a Querier whose driver errors classify to ErrSerialization.
type classifiedQ struct{ loadQ }

func (q classifiedQ) conn() *execer {
	return &execer{d: q.d, classify: func(err error) error { return &Error{Kind: ErrSerialization, Err: err} }}
}

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
	Table:      "kids",
	SoftDelete: -1,
	Columns:    []ColumnMeta{{Name: "pid"}, {Name: "seq"}},
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

func argRange(n int) []int {
	vs := make([]int, n)
	for i := range vs {
		vs[i] = 1000 + i
	}
	return vs
}

// TestLoadReservesUserScopeArgs pins that the arguments of LoadWhere
// predicates are reserved out of the bind budget: with 20 scope args and a
// limit of 40, only 4 keys fit per statement.
func TestLoadReservesUserScopeArgs(t *testing.T) {
	d := *dialect.For(dialect.Postgres)
	d.MaxParams = 40
	seq := NewColumn[int]("kids", "seq", 0)
	var cfg loadConfig
	LoadWhere[kid](seq.In(argRange(20)...))(&cfg)
	parents := make([]par, 30)
	for i := range parents {
		parents[i].Pid = i + 1
	}
	rel := kidsRel(HasMany)
	pl, err := rel.prepareWith(loadQ{&d}, parents, &cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	qs := pl.(*pending[par, kid]).queries
	if len(qs) != 8 {
		t.Fatalf("30 keys at (40-16-20 floored to) 4 per statement: want 8 statements, got %d", len(qs))
	}
	for i, q := range qs {
		if len(q.Args) > d.MaxParams {
			t.Fatalf("statement %d binds %d > limit %d", i, len(q.Args), d.MaxParams)
		}
	}
}

func TestLoadScopeArgsExceedingBindLimitFail(t *testing.T) {
	d := *dialect.For(dialect.Postgres)
	d.MaxParams = 40
	seq := NewColumn[int]("kids", "seq", 0)
	var cfg loadConfig
	LoadWhere[kid](seq.In(argRange(40)...))(&cfg)
	rel := kidsRel(HasMany)
	_, err := rel.prepareWith(loadQ{&d}, []par{{Pid: 1}}, &cfg, nil)
	var me *Error
	if !errors.As(err, &me) || me.Kind != ErrValidation || me.Op != "load" {
		t.Fatalf("want ErrValidation with Op load, got %v", err)
	}
}

func TestLoadPerParentLimitRejectsToOneAndNegative(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind RelationKind
		n    int
		want error
	}{
		{"belongs-to", BelongsTo, 3, ErrUnsupported},
		{"has-one", HasOne, 3, ErrUnsupported},
		{"negative", HasMany, -1, ErrValidation},
	} {
		var cfg loadConfig
		LoadPerParentLimit[kid](tc.n)(&cfg)
		rel := kidsRel(tc.kind)
		_, err := rel.prepareWith(loadQ{dialect.For(dialect.Postgres)}, []par{{Pid: 1}}, &cfg, nil)
		var me *Error
		if !errors.As(err, &me) || me.Kind != tc.want || me.Op != "load" {
			t.Fatalf("%s: want %v with Op load, got %v", tc.name, tc.want, err)
		}
	}
}

func topNSQL(t *testing.T, rel Relation[par, kid], parents []par, n int, opts ...LoadOption[kid]) []Compiled {
	t.Helper()
	var cfg loadConfig
	for _, o := range opts {
		o(&cfg)
	}
	LoadPerParentLimit[kid](n)(&cfg)
	pl, err := rel.prepareWith(loadQ{dialect.For(dialect.Postgres)}, parents, &cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	return pl.(*pending[par, kid]).queries
}

func TestLoadTopNStatementShape(t *testing.T) {
	qs := topNSQL(t, kidsRel(HasMany), []par{{Pid: 1}, {Pid: 2}, {Pid: -1}, {Pid: 1}}, 3)
	if len(qs) != 1 {
		t.Fatalf("one statement for two distinct keys (NULL skipped), got %d", len(qs))
	}
	sql := qs[0].SQL
	for _, want := range []string{
		`ROW_NUMBER() OVER (PARTITION BY "kids"."pid" ORDER BY "kids"."pid", "kids"."seq") AS "momo_rn"`,
		`WHERE "momo_t"."momo_rn" <= $`,
		`ORDER BY "momo_t"."momo_rn", "momo_t"."pid"`,
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("missing %q in\n%s", want, sql)
		}
	}
	if last := qs[0].Args[len(qs[0].Args)-1]; last != int64(3) {
		t.Fatalf("limit must be the last bind, got %v", qs[0].Args)
	}
}

func TestLoadTopNNoKeysNoStatement(t *testing.T) {
	if qs := topNSQL(t, kidsRel(HasMany), []par{{Pid: -1}}, 2); len(qs) != 0 {
		t.Fatalf("NULL keys must issue no statement: %v", qs)
	}
}

func TestLoadTopNReservesLimitBind(t *testing.T) {
	d := *dialect.For(dialect.Postgres)
	d.MaxParams = 20 // reserved 16 + the limit bind leaves room for 3 keys
	parents := make([]par, 7)
	for i := range parents {
		parents[i].Pid = i
	}
	var cfg loadConfig
	LoadPerParentLimit[kid](2)(&cfg)
	rel := kidsRel(HasMany)
	pl, err := rel.prepareWith(loadQ{&d}, parents, &cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range pl.(*pending[par, kid]).queries {
		if len(q.Args) > d.MaxParams {
			t.Fatalf("statement binds %d > %d", len(q.Args), d.MaxParams)
		}
	}
}

// softKidMeta is kidMeta with a timestamp soft-delete marker (column "gone").
var softKidMeta = Meta[kid]{
	Table:          "kids",
	SoftDelete:     2,
	SoftDeleteKind: SoftDeleteTimestamp,
	Columns:        []ColumnMeta{{Name: "pid"}, {Name: "seq"}, {Name: "gone", Flags: FlagSoftDelete | FlagNullable}},
	Ptr:            kidMeta.Ptr,
}

func softKidsSQL(t *testing.T, opts ...LoadOption[kid]) string {
	t.Helper()
	rel := kidsRel(HasMany)
	rel.ChildMeta = &softKidMeta
	var cfg loadConfig
	for _, o := range opts {
		o(&cfg)
	}
	pl, err := rel.prepareWith(loadQ{dialect.For(dialect.Postgres)}, []par{{Pid: 1}}, &cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	return pl.(*pending[par, kid]).queries[0].SQL
}

func TestLoadExcludesSoftDeletedChildrenByDefault(t *testing.T) {
	const want = `SELECT "kids"."pid", "kids"."seq", "kids"."gone" FROM "kids" WHERE "kids"."pid" IN ($1) AND "kids"."gone" IS NULL ORDER BY "kids"."pid", "kids"."seq"`
	if got := softKidsSQL(t); got != want {
		t.Fatalf("default load must hide soft-deleted children:\n got %s\nwant %s", got, want)
	}
	if got := softKidsSQL(t, LoadWithDeleted[kid]()); strings.Contains(got, "gone\" IS") {
		t.Fatalf("LoadWithDeleted must not filter: %s", got)
	}
	if got := softKidsSQL(t, LoadOnlyDeleted[kid]()); !strings.Contains(got, `"kids"."gone" IS NOT NULL`) {
		t.Fatalf("LoadOnlyDeleted must select deleted rows: %s", got)
	}
	if got := softKidsSQL(t, LoadOnlyDeleted[kid](), LoadWithDeleted[kid]()); strings.Contains(got, "gone\" IS") {
		t.Fatalf("the last of WithDeleted/OnlyDeleted wins, as in Select: %s", got)
	}
}

// Parent and child key columns of different integer widths must group
// together: the parent holds an int32, the child scans an int64.
type par32 struct {
	ID   int32
	Kids []kid64
}
type kid64 struct{ PID, Seq int64 }

var kid64Meta = Meta[kid64]{
	Table: "kids", SoftDelete: -1,
	Columns: []ColumnMeta{{Name: "pid"}, {Name: "seq"}},
	Ptr: func(m *kid64, i int) any {
		if i == 0 {
			return &m.PID
		}
		return &m.Seq
	},
}

type kid64Rows struct {
	rows []kid64
	i    int
}

func (r *kid64Rows) Next() bool { r.i++; return r.i <= len(r.rows) }
func (r *kid64Rows) Scan(dest ...any) error {
	*(dest[0].(*int64)), *(dest[1].(*int64)) = r.rows[r.i-1].PID, r.rows[r.i-1].Seq
	return nil
}
func (r *kid64Rows) Err() error   { return nil }
func (r *kid64Rows) Close() error { return nil }

func TestLoadGroupsAcrossKeyIntegerWidths(t *testing.T) {
	rel := Relation[par32, kid64]{
		Name: "Kids", Kind: HasMany,
		Parent: TableRef{Name: "pars"}, Child: TableRef{Name: "kids"},
		ChildMeta:  &kid64Meta,
		ParentCols: []string{"id"}, ChildCols: []string{"pid"}, ChildPK: []string{"seq"},
		ParentKey: func(p *par32) ([]any, bool) { return []any{p.ID}, true },
		ChildKey:  func(c *kid64) ([]any, bool) { return []any{c.PID}, true },
		GetMany:   func(p *par32) []kid64 { return p.Kids },
		SetMany:   func(p *par32, cs []kid64) { p.Kids = cs },
	}
	parents := []par32{{ID: 7}}
	pl, err := rel.prepareWith(loadQ{dialect.For(dialect.Postgres)}, parents, &loadConfig{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	pd := pl.(*pending[par32, kid64])
	if err := pd.consume(&kid64Rows{rows: []kid64{{7, 1}, {7, 2}}}); err != nil {
		t.Fatal(err)
	}
	if err := pd.finish(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(parents[0].Kids) != 2 {
		t.Fatalf("int32 parent key must match int64 child FK, got %v", parents[0].Kids)
	}
}

type errRows struct {
	sliceRows
	scanErr, iterErr error
}

func (r *errRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	return r.sliceRows.Scan(dest...)
}
func (r *errRows) Err() error { return r.iterErr }

func TestLoadWrapsScanAndIterationErrors(t *testing.T) {
	for name, rows := range map[string]*errRows{
		"scan": {sliceRows: sliceRows{rows: []kid{{1, 1}}}, scanErr: errors.New("boom")},
		"iter": {iterErr: errors.New("boom")},
	} {
		rel := kidsRel(HasMany)
		pl, perr := rel.prepareWith(classifiedQ{loadQ{dialect.For(dialect.Postgres)}}, []par{{Pid: 1}}, &loadConfig{}, nil)
		if perr != nil {
			t.Fatal(perr)
		}
		err := pl.(*pending[par, kid]).consume(rows)
		var me *Error
		if !errors.As(err, &me) || me.Op != "load" || me.Table != "kids" || !strings.Contains(err.Error(), "boom") {
			t.Fatalf("%s: want classified *Error Op load Table kids wrapping the cause, got %#v", name, err)
		}
	}
}

// Catches: a select-list alias rendered inside the per-parent window ORDER BY,
// where an output alias is invalid.
func TestLoadPerParentLimitRejectsAliasOrder(t *testing.T) {
	var cfg loadConfig
	LoadOrderBy[kid](CountAll().As("n").Asc())(&cfg)
	LoadPerParentLimit[kid](2)(&cfg)
	rel := kidsRel(HasMany)
	_, err := rel.prepareWith(loadQ{dialect.For(dialect.Postgres)}, []par{{Pid: 1}}, &cfg, nil)
	var me *Error
	if !errors.As(err, &me) || me.Kind != ErrValidation {
		t.Fatalf("want ErrValidation, got %v", err)
	}
}
