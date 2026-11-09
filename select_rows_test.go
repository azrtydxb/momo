// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

// checkTupleMeta asserts the metadata contract of one tuple instantiation:
// arity columns, Ptr aimed at the matching field, no writable surface.
func checkTupleMeta[T any, PT momo.Modeler[T]](t *testing.T, n int, fields func(*T) []any) {
	t.Helper()
	var p PT
	m := p.MomoMeta()
	if m != p.MomoMeta() {
		t.Errorf("%T: MomoMeta returned two different pointers", *new(T))
	}
	if len(m.Columns) != n {
		t.Fatalf("%T: %d columns, want %d", *new(T), len(m.Columns), n)
	}
	var row T
	fs := fields(&row)
	for i := range n {
		if want := "c" + strconv.Itoa(i+1); m.Columns[i].Name != want {
			t.Errorf("%T: column %d named %q, want %q", row, i, m.Columns[i].Name, want)
		}
		if got := m.Ptr(&row, i); got != fs[i] {
			t.Errorf("%T: Ptr(%d) = %v, want the address of C%d", row, i, got, i+1)
		}
		if m.Value(&row, i) != nil {
			t.Errorf("%T: Value(%d) is not nil", row, i)
		}
	}
	if m.Ptr(&row, n) != nil {
		t.Errorf("%T: Ptr past the last column is not nil", row)
	}
	if m.Table != "" || m.Schema != "" || m.TableSchema != nil || len(m.PK) != 0 || len(m.Fields) != 0 {
		t.Errorf("%T: a tuple must have no table, schema, primary key or field hooks: %+v", row, m)
	}
	if m.Identity != -1 || m.Version != -1 || m.SoftDelete != -1 || m.CreatedAt != -1 || m.UpdatedAt != -1 {
		t.Errorf("%T: derived column indexes must be -1: %+v", row, m)
	}
}

func TestTupleRowMeta(t *testing.T) {
	checkTupleMeta[momo.Row1[int]](t, 1, func(r *momo.Row1[int]) []any { return []any{&r.C1} })
	checkTupleMeta[momo.Row2[int, string]](t, 2, func(r *momo.Row2[int, string]) []any { return []any{&r.C1, &r.C2} })
	checkTupleMeta[momo.Row3[int, string, bool]](t, 3, func(r *momo.Row3[int, string, bool]) []any {
		return []any{&r.C1, &r.C2, &r.C3}
	})
	checkTupleMeta[momo.Row4[int, string, bool, *int64]](t, 4, func(r *momo.Row4[int, string, bool, *int64]) []any {
		return []any{&r.C1, &r.C2, &r.C3, &r.C4}
	})
	checkTupleMeta[momo.Row5[int, int, int, int, string]](t, 5, func(r *momo.Row5[int, int, int, int, string]) []any {
		return []any{&r.C1, &r.C2, &r.C3, &r.C4, &r.C5}
	})
	checkTupleMeta[momo.Row6[int, int, int, int, int, string]](t, 6, func(r *momo.Row6[int, int, int, int, int, string]) []any {
		return []any{&r.C1, &r.C2, &r.C3, &r.C4, &r.C5, &r.C6}
	})
	checkTupleMeta[momo.Row7[int, int, int, int, int, int, string]](t, 7, func(r *momo.Row7[int, int, int, int, int, int, string]) []any {
		return []any{&r.C1, &r.C2, &r.C3, &r.C4, &r.C5, &r.C6, &r.C7}
	})
	checkTupleMeta[momo.Row8[int, int, int, int, int, int, int, string]](t, 8, func(r *momo.Row8[int, int, int, int, int, int, int, string]) []any {
		return []any{&r.C1, &r.C2, &r.C3, &r.C4, &r.C5, &r.C6, &r.C7, &r.C8}
	})
	checkTupleMeta[momo.Row9[int, int, int, int, int, int, int, int, string]](t, 9, func(r *momo.Row9[int, int, int, int, int, int, int, int, string]) []any {
		return []any{&r.C1, &r.C2, &r.C3, &r.C4, &r.C5, &r.C6, &r.C7, &r.C8, &r.C9}
	})
	checkTupleMeta[momo.Row10[int, int, int, int, int, int, int, int, int, string]](t, 10, func(r *momo.Row10[int, int, int, int, int, int, int, int, int, string]) []any {
		return []any{&r.C1, &r.C2, &r.C3, &r.C4, &r.C5, &r.C6, &r.C7, &r.C8, &r.C9, &r.C10}
	})
}

// Concurrent first use of a fresh instantiation must still hand every caller
// the same *Meta, and the cached lookup must not allocate.
func TestTupleRowMetaConcurrentAndAllocFree(t *testing.T) {
	type fresh = momo.Row3[uint8, int16, float32]
	var p *fresh
	const n = 32
	got := make([]*momo.Meta[fresh], n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			got[i] = p.MomoMeta()
		}()
	}
	close(start)
	wg.Wait()
	for i := range got {
		if got[i] == nil || got[i] != got[0] {
			t.Fatalf("goroutine %d got a different Meta pointer", i)
		}
	}
	if a := testing.AllocsPerRun(100, func() { _ = p.MomoMeta() }); a != 0 {
		t.Errorf("cached MomoMeta allocates %v times per call, want 0", a)
	}
}

func TestTupleRowScansPositionally(t *testing.T) {
	fk := fakedriver.New(dialect.Postgres)
	db := momo.New(fk)
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	tsp := &ts
	fk.SetRows([][]any{
		{"a", int64(3), nil},
		{"b", int64(4), tsp},
	})
	rows, err := momo.Select[momo.Row3[string, int64, *time.Time]](db).
		From(momo.TableRef{Name: "documents"}).
		Columns(DocTitle, DocTenantID, DocDeletedAt).
		All(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0].C1 != "a" || rows[0].C2 != 3 || rows[0].C3 != nil {
		t.Errorf("row 0 = %+v", rows[0])
	}
	if rows[1].C1 != "b" || rows[1].C2 != 4 || rows[1].C3 == nil || !rows[1].C3.Equal(ts) {
		t.Errorf("row 1 = %+v", rows[1])
	}
}

// A select list whose length differs from the tuple's arity is a validation
// error before any statement runs, never a scan panic or a silent mis-scan.
func TestTupleRowColumnCountMismatch(t *testing.T) {
	fk := fakedriver.New(dialect.Postgres)
	db := momo.New(fk)
	fk.SetRows([][]any{{"a", int64(1)}})
	docs := momo.TableRef{Name: "documents"}
	ctx := context.Background()
	cases := map[string]momo.SelectQuery[momo.Row2[string, int64]]{
		"too few":    momo.Select[momo.Row2[string, int64]](db).From(docs).Columns(DocTitle),
		"too many":   momo.Select[momo.Row2[string, int64]](db).From(docs).Columns(DocTitle, DocTenantID, DocID),
		"no columns": momo.Select[momo.Row2[string, int64]](db).From(docs),
		"empty":      momo.Select[momo.Row2[string, int64]](db).From(docs).Columns(),
	}
	for name, q := range cases {
		if _, err := q.SQL(); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: SQL() = %v, want ErrValidation", name, err)
		}
		if _, err := q.All(ctx); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: All() = %v, want ErrValidation", name, err)
		}
		if _, err := q.One(ctx); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: One() = %v, want ErrValidation", name, err)
		}
		if err := q.Each(ctx, func(*momo.Row2[string, int64]) error { return nil }); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: Each() = %v, want ErrValidation", name, err)
		}
		for _, err := range q.Iter(ctx) {
			if !errors.Is(err, momo.ErrValidation) {
				t.Errorf("%s: Iter() = %v, want ErrValidation", name, err)
			}
		}
		if _, err := q.Count(ctx); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: Count() = %v, want ErrValidation", name, err)
		}
	}
	// A mismatched tuple used as an inner subquery fails the outer statement.
	inner := momo.Select[momo.Row2[string, int64]](db).From(docs).Columns(DocTitle)
	outer := momo.Select[momo.Row1[string]](db).From(docs).Columns(DocTitle).Where(DocTitle.InQuery(inner))
	if _, err := outer.SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("inner subquery: SQL() = %v, want ErrValidation", err)
	}
	if _, err := outer.All(ctx); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("inner subquery: All() = %v, want ErrValidation", err)
	}
	if _, err := momo.Select[momo.Row2[string, int64]](db).From(docs).Columns(DocTitle).SQL(); err == nil ||
		!strings.Contains(err.Error(), "1 expressions") || !strings.Contains(err.Error(), "maps 2 fields") {
		t.Errorf("mismatch error does not name both counts: %v", err)
	}
	if n := len(fk.Calls()); n != 0 {
		t.Errorf("%d statements reached the driver, want none", n)
	}
}

// Tuple rows are projections: they have no table, so every write is refused.
func TestTupleRowWritesRefused(t *testing.T) {
	fk := fakedriver.New(dialect.Postgres)
	db := momo.New(fk)
	ctx := context.Background()
	type row = momo.Row2[string, int64]
	r := row{C1: "a", C2: 1}

	if _, err := momo.Insert[row](db).Model(&r).SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("Insert SQL = %v, want ErrValidation", err)
	}
	if _, err := momo.Insert[row](db).Model(&r).Exec(ctx); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("Insert Exec = %v, want ErrValidation", err)
	}
	if _, err := momo.Insert[row](db).Models([]row{r}).Exec(ctx); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("Insert Models Exec = %v, want ErrValidation", err)
	}
	if _, err := momo.Upsert[row](db).Models([]row{r}).Exec(ctx); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("Upsert Exec = %v, want ErrValidation", err)
	}
	if _, err := momo.SoftDelete[row](db).Where(DocID.Eq(1)).Exec(ctx); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("SoftDelete Exec = %v, want ErrValidation", err)
	}
	if _, err := momo.ForceDelete[row](db).Where(DocID.Eq(1)).Exec(ctx); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("ForceDelete Exec = %v, want ErrValidation", err)
	}
	if _, err := momo.Update[row](db).Set(DocTitle.Set("x")).Where(DocID.Eq(1)).Exec(ctx); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("Update Exec = %v, want ErrValidation", err)
	}
	if _, err := momo.Delete[row](db).Where(DocID.Eq(1)).Exec(ctx); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("Delete Exec = %v, want ErrValidation", err)
	}
	if n := len(fk.Calls()); n != 0 {
		t.Errorf("%d statements reached the driver, want none", n)
	}
}

func TestTupleRowGolden(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	docs := momo.TableRef{Name: "documents"}
	left := momo.TableRef{Name: "documents", Alias: "l"}
	right := momo.TableRef{Name: "documents", Alias: "r"}

	maxTenant := momo.Select[momo.Row1[int64]](pg).From(docs).Columns(momo.Max(DocTenantID))

	byTenant := func(id int64) momo.SelectQuery[momo.Row2[int64, string]] {
		return momo.Select[momo.Row2[int64, string]](pg).From(docs).Columns(DocTenantID, DocTitle).Where(DocTenantID.Eq(id))
	}
	title := momo.NewColumn[string]("docs", "title", 0)

	cases := []struct {
		name string
		q    interface{ SQL() (momo.Compiled, error) }
		want string
	}{
		{"aggregate", momo.Select[momo.Row2[int64, int64]](pg).From(docs).
			Columns(DocTenantID, momo.CountAll()).GroupBy(DocTenantID).Having(momo.CountAll().Gt(1)),
			`SELECT "documents"."tenant_id", COUNT(*) FROM "documents" GROUP BY "documents"."tenant_id" HAVING COUNT(*) > $1`},
		{"single column", momo.Select[momo.Row1[int64]](pg).From(docs).Columns(momo.CountAll()),
			`SELECT COUNT(*) FROM "documents"`},
		{"left join nullable", momo.Select[momo.Row3[string, *int64, *string]](pg).From(left).
			Columns(DocTitle.As("l"), DocID.As("r"), DocTitle.As("r")).
			LeftJoin(right, DocTenantID.As("l").EqExpr(DocTenantID.As("r")), DocTitle.As("r").Eq("b")).
			OrderBy(DocTitle.As("l").Asc()),
			`SELECT "l"."title", "r"."id", "r"."title" FROM "documents" AS "l" LEFT JOIN "documents" AS "r" ON ("l"."tenant_id" = "r"."tenant_id") AND ("r"."title" = $1) ORDER BY "l"."title" ASC`},
		{"subquery column", momo.Select[momo.Row2[string, int64]](pg).From(docs).Columns(DocTitle, maxTenant),
			`SELECT "documents"."title", (SELECT MAX("documents"."tenant_id") FROM "documents") FROM "documents"`},
		{"union as source", momo.Select[momo.Row2[int64, string]](pg).
			From(momo.FromSubquery(momo.Union(true, byTenant(1), byTenant(2)), "u")).
			Columns(momo.ColumnOf[int64]("u", "tenant_id"), momo.ColumnOf[string]("u", "title")),
			`SELECT "u"."tenant_id", "u"."title" FROM (SELECT "documents"."tenant_id", "documents"."title" FROM "documents" WHERE "documents"."tenant_id" = $1 UNION ALL SELECT "documents"."tenant_id", "documents"."title" FROM "documents" WHERE "documents"."tenant_id" = $2) AS "u"`},
		{"cte", momo.Select[momo.Row1[string]](pg).
			With("big", momo.Select[momo.Row2[int64, string]](pg).From(docs).Columns(DocTenantID, DocTitle).Where(DocTenantID.Ge(2)), "tenant_id", "title").
			From(momo.CTE("big")).Columns(momo.ColumnOf[string]("big", "title")),
			`WITH "big" ("tenant_id", "title") AS (SELECT "documents"."tenant_id", "documents"."title" FROM "documents" WHERE "documents"."tenant_id" >= $1) SELECT "big"."title" FROM "big"`},
		// No filter of its own; the From table's soft-delete column applies as
		// for any projection, and WithDeleted lifts it.
		{"plain table has no filter", momo.Select[momo.Row1[string]](pg).From(momo.TableRef{Name: "docs"}).Columns(title),
			`SELECT "docs"."title" FROM "docs"`},
		{"soft-delete table filters", momo.Select[momo.Row1[string]](pg).From(momo.TableRef{Name: "docs", SoftDelete: "deleted_at"}).Columns(title),
			`SELECT "docs"."title" FROM "docs" WHERE "docs"."deleted_at" IS NULL`},
		{"with deleted", momo.Select[momo.Row1[string]](pg).From(momo.TableRef{Name: "docs", SoftDelete: "deleted_at"}).Columns(title).WithDeleted(),
			`SELECT "docs"."title" FROM "docs"`},
	}
	// One case per arity above 3: n copies of the title column.
	titles := func(n int) []momo.Expr {
		es := make([]momo.Expr, n)
		for i := range es {
			es[i] = DocTitle
		}
		return es
	}
	wide := map[int]interface{ SQL() (momo.Compiled, error) }{
		4:  momo.Select[momo.Row4[string, string, string, string]](pg).From(docs).Columns(titles(4)...),
		5:  momo.Select[momo.Row5[string, string, string, string, string]](pg).From(docs).Columns(titles(5)...),
		6:  momo.Select[momo.Row6[string, string, string, string, string, string]](pg).From(docs).Columns(titles(6)...),
		7:  momo.Select[momo.Row7[string, string, string, string, string, string, string]](pg).From(docs).Columns(titles(7)...),
		8:  momo.Select[momo.Row8[string, string, string, string, string, string, string, string]](pg).From(docs).Columns(titles(8)...),
		9:  momo.Select[momo.Row9[string, string, string, string, string, string, string, string, string]](pg).From(docs).Columns(titles(9)...),
		10: momo.Select[momo.Row10[string, string, string, string, string, string, string, string, string, string]](pg).From(docs).Columns(titles(10)...),
	}
	for n, q := range wide {
		cols := strings.TrimSuffix(strings.Repeat(`"documents"."title", `, n), ", ")
		cases = append(cases, struct {
			name string
			q    interface{ SQL() (momo.Compiled, error) }
			want string
		}{"Row" + strconv.Itoa(n), q, `SELECT ` + cols + ` FROM "documents"`})
	}
	for _, c := range cases {
		got, err := c.q.SQL()
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got.SQL != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got.SQL, c.want)
		}
	}
}

// A tuple has no primary key, so After keys on the OrderBy items alone.
func TestTupleRowKeyset(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	base := momo.Select[momo.Row2[int64, string]](pg).From(momo.TableRef{Name: "documents"}).
		Columns(DocTenantID, DocTitle).OrderBy(DocTenantID.Asc())
	c, err := base.After(int64(1)).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.SQL, `"documents"."tenant_id" > $1`) || !strings.HasSuffix(c.SQL, `ORDER BY "documents"."tenant_id" ASC`) {
		t.Errorf("keyset SQL = %s", c.SQL)
	}
	if _, err := base.After(int64(1), "x").SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("After with a value per column but one ORDER BY key = %v, want ErrValidation", err)
	}
}
