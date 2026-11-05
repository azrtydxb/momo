// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
)

// pair is a projection of two aliased title columns.
type pair struct {
	momo.Projection
	L, R string
}

func (*pair) MomoMeta() *momo.Meta[pair] {
	return &momo.Meta[pair]{
		Columns:    []momo.ColumnMeta{{Name: "l_title"}, {Name: "r_title"}},
		Ptr:        func(m *pair, i int) any { return []any{&m.L, &m.R}[i] },
		Value:      func(m *pair, i int) any { return []any{m.L, m.R}[i] },
		Identity:   -1,
		Version:    -1,
		SoftDelete: -1,
		CreatedAt:  -1,
		UpdatedAt:  -1,
	}
}

// tenantCount is a projection of a tenant id and a row count.
type tenantCount struct {
	momo.Projection
	TenantID int64
	N        int64
}

func (*tenantCount) MomoMeta() *momo.Meta[tenantCount] {
	return &momo.Meta[tenantCount]{
		Columns: []momo.ColumnMeta{{Name: "tenant_id"}, {Name: "n"}},
		Ptr:     func(m *tenantCount, i int) any { return []any{&m.TenantID, &m.N}[i] },
		Value:   func(m *tenantCount, i int) any { return nil },
	}
}

// intRow is a one-integer projection.
type intRow struct {
	momo.Projection
	N int64
}

func (*intRow) MomoMeta() *momo.Meta[intRow] {
	return &momo.Meta[intRow]{
		Columns: []momo.ColumnMeta{{Name: "n"}},
		Ptr:     func(m *intRow, _ int) any { return &m.N },
		Value:   func(m *intRow, _ int) any { return nil },
	}
}

// seedDocs inserts one document per (tenant, title) pair.
func seedDocs(t *testing.T, db *momo.DB, rows ...[2]any) {
	t.Helper()
	for _, r := range rows {
		if _, err := momo.Exec(context.Background(), db, "INSERT INTO documents (tenant_id, title) VALUES (?, ?)", r[0], r[1]); err != nil {
			t.Fatal(err)
		}
	}
}

// JoinsAliasesSelfJoin asserts that a self-join under two aliases pairs each
// row with its tenant peers and never with itself.
func JoinsAliasesSelfJoin(t *testing.T, db *momo.DB, caps Caps) {
	CreateDocuments(t, db, caps)
	ctx := context.Background()
	seedDocs(t, db, [2]any{1, "a"}, [2]any{1, "b"}, [2]any{2, "c"})
	left := momo.TableRef{Name: "documents", Alias: "l"}
	right := momo.TableRef{Name: "documents", Alias: "r"}
	rows, err := momo.Select[pair](db).
		From(left).
		Columns(DocTitle.As("l").AsExpr(), DocTitle.As("r").AsExpr()).
		Join(right, DocTenantID.As("l").EqExpr(DocTenantID.As("r")), DocID.As("l").NeExpr(DocID.As("r"))).
		OrderBy(DocTitle.As("l").Asc(), DocTitle.As("r").Asc()).
		All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].L != "a" || rows[0].R != "b" || rows[1].L != "b" || rows[1].R != "a" {
		t.Fatalf("self-join rows = %+v, want (a,b) and (b,a)", rows)
	}
}

// AggregatesGroupHaving asserts GROUP BY with a HAVING over COUNT(*).
func AggregatesGroupHaving(t *testing.T, db *momo.DB, caps Caps) {
	CreateDocuments(t, db, caps)
	seedDocs(t, db, [2]any{1, "x"}, [2]any{1, "x"}, [2]any{1, "x"}, [2]any{2, "x"})
	rows, err := momo.Select[tenantCount](db).
		From(momo.TableRef{Name: "documents"}).
		Columns(DocTenantID, momo.CountAll()).
		GroupBy(DocTenantID).
		Having(momo.CountAll().Gt(1)).
		OrderBy(DocTenantID.Asc()).
		All(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].TenantID != 1 || rows[0].N != 3 {
		t.Fatalf("rows = %+v, want tenant 1 with 3 documents", rows)
	}
}

// SubqueriesCTEsRecursive asserts an IN subquery and a recursive CTE, through
// the builder and through raw SQL.
func SubqueriesCTEsRecursive(t *testing.T, db *momo.DB, caps Caps) {
	CreateDocuments(t, db, caps)
	ctx := context.Background()
	seedDocs(t, db, [2]any{1, "x"}, [2]any{2, "x"}, [2]any{3, "x"})

	sub := momo.Select[intRow](db).From(momo.TableRef{Name: "documents"}).Columns(DocTenantID).Where(DocTenantID.Ge(2))
	n, err := momo.Select[Document](db).Where(DocTenantID.InQuery(sub)).Count(ctx)
	if err != nil || n != 2 {
		t.Fatalf("IN subquery count = %d, %v; want 2", n, err)
	}

	seqN := momo.ColumnOf[int64]("seq", "n")
	body := momo.Union(true,
		momo.RawSubquery(`SELECT 1 AS n`),
		momo.Select[intRow](db).From(momo.CTE("seq")).
			Columns(momo.TypedSQL[int64](`"seq"."n" + 1`)).Where(seqN.Lt(5)),
	)
	rows, err := momo.Select[intRow](db).
		WithRecursive("seq", body, "n").
		From(momo.CTE("seq")).
		Columns(seqN).
		OrderBy(seqN.Asc()).
		All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 5 || rows[0].N != 1 || rows[4].N != 5 {
		t.Fatalf("recursive CTE returned %+v", rows)
	}

	ints, err := momo.Scalars[int64](ctx, db, `WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n + 1 FROM seq WHERE n < 5) SELECT n FROM seq ORDER BY n`)
	if err != nil || len(ints) != 5 || ints[4] != 5 {
		t.Fatalf("Scalars over recursive CTE = %v, %v", ints, err)
	}
}

// ReturningRendersAndScans asserts that INSERT ... RETURNING renders and scans.
// The builder has no RETURNING refusal for a dialect without it yet, so a
// dialect with caps.Returning false is momo-08's to add; PostgreSQL and SQLite
// both support it.
func ReturningRendersAndScans(t *testing.T, db *momo.DB, caps Caps) {
	if !caps.Returning {
		t.Skip("dialect has no RETURNING")
	}
	CreateDocuments(t, db, caps)
	ctx := context.Background()
	q := momo.Insert[Document](db).
		Values(DocTenantID.Set(7), DocTitle.Set("r")).
		Returning(DocID, DocTitle)
	rows, err := q.Rows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var doc Document
	if !rows.Next() {
		t.Fatal("no RETURNING row")
	}
	if err := rows.ScanModel(&doc); err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if doc.ID == 0 || doc.Title != "r" {
		t.Fatalf("returned %+v", doc)
	}
}

// RawArgs asserts raw SQL arguments end to end: a named argument, an expanded
// IN list, an identifier argument, a literal ? inside a string literal and a
// comment (never a placeholder), and the build error for an empty raw IN list.
func RawArgs(t *testing.T, db *momo.DB, caps Caps) {
	CreateDocuments(t, db, caps)
	ctx := context.Background()
	seedDocs(t, db, [2]any{1, "a"}, [2]any{1, "b"}, [2]any{1, "c"})

	n, err := momo.Exec(ctx, db,
		"UPDATE documents SET secret = @s WHERE title IN (?)",
		momo.Named("s", "x"), momo.In([]string{"a", "b"}))
	if err != nil || n != 2 {
		t.Fatalf("named + In update n=%d err=%v", n, err)
	}

	rows, err := momo.Select[Document](db).
		Where(momo.TypedSQL[bool](`? = ?`, momo.Ident("title"), "a")).
		All(ctx)
	if err != nil || len(rows) != 1 || rows[0].Title != "a" {
		t.Fatalf("Ident compares the column, not a value: rows=%d err=%v", len(rows), err)
	}

	// A ? inside a string literal and a comment is data, not a placeholder.
	got, err := momo.Scalars[string](ctx, db,
		"SELECT 'what?' /* why ? */ FROM documents WHERE title = ? -- ?\n", "c")
	if err != nil || len(got) != 1 || got[0] != "what?" {
		t.Fatalf("literal ? = %v, %v", got, err)
	}

	secrets, err := momo.Scalars[string](ctx, db,
		"SELECT secret FROM documents WHERE title NOT IN (?) ORDER BY title", momo.In([]string{"a", "b"}))
	if err != nil || len(secrets) != 1 || secrets[0] != "" {
		t.Fatalf("NOT IN expansion = %v, %v", secrets, err)
	}

	_, err = momo.Exec(ctx, db, "DELETE FROM documents WHERE title NOT IN (?)", momo.In([]string{}))
	var me *momo.Error
	if !errors.As(err, &me) || me.Kind != momo.ErrValidation {
		t.Fatalf("empty raw IN err = %v, want a validation build error", err)
	}
	cnt, err := momo.Select[Document](db).Count(ctx)
	if err != nil || cnt != 3 {
		t.Fatalf("empty IN must not have executed: count=%d err=%v", cnt, err)
	}
}
