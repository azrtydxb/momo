// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
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
			Columns(momo.TypedSQL[int64](`seq.n + 1`)).Where(seqN.Lt(5)),
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
	for rows.Next() { // read to the end, as a write on an implicit-transaction handle needs
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
	rawDollarPlaceholders(t, db, caps)
}

// rawDollarPlaceholders asserts native $n placeholders in raw SQL: PostgreSQL
// binds a reused position once, renumbers it after prior binds, and runs the
// statement; every other dialect rejects $n with ErrUnsupported at build time.
func rawDollarPlaceholders(t *testing.T, db *momo.DB, caps Caps) {
	t.Helper()
	ctx := context.Background()
	if caps.Dialect != dialect.Postgres {
		_, err := momo.Scalars[string](ctx, db, "SELECT title FROM documents WHERE title = $1", "a")
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("$n off PostgreSQL: err = %v, want ErrUnsupported", err)
		}
		return
	}
	got, err := momo.Scalars[string](ctx, db,
		"SELECT title FROM documents WHERE title = $2 OR title = $1 OR title = $2 ORDER BY title", "a", "c")
	if err != nil || len(got) != 2 || got[0] != "a" || got[1] != "c" {
		t.Fatalf("$n reused = %v, %v", got, err)
	}
	// Embedded after a WHERE bind: the fragment's $1 is statement bind $2.
	rows, err := momo.Select[Document](db).
		Where(DocTenantID.Eq(1)).
		Where(momo.SQL("(title = $2 OR title = $1)", "a", "b")).
		OrderBy(DocTitle.Asc()).All(ctx)
	if err != nil || len(rows) != 2 {
		t.Fatalf("$n after a bind: rows=%d err=%v", len(rows), err)
	}
	n, err := momo.Scalars[int64](ctx, db,
		"SELECT count(*) FROM documents WHERE title IN (SELECT title FROM documents WHERE tenant_id = $1) AND tenant_id = $1", int64(1))
	if err != nil || len(n) != 1 || n[0] != 3 {
		t.Fatalf("$n reused in a subquery = %v, %v", n, err)
	}
	var me *momo.Error
	if _, err := momo.Exec(ctx, db, "UPDATE documents SET title = $1 WHERE id = ?", "x", 1); !errors.As(err, &me) || me.Kind != momo.ErrValidation {
		t.Fatalf("mixed styles err = %v, want ErrValidation", err)
	}
	if _, err := momo.Exec(ctx, db, "UPDATE documents SET title = $1 WHERE id = $3", "x", 1); !errors.As(err, &me) || me.Kind != momo.ErrValidation {
		t.Fatalf("out of range $n err = %v, want ErrValidation", err)
	}
}

// LockingNowaitSkipLocked asserts row locking: with two transactions, SKIP
// LOCKED passes over a row the other transaction holds and NOWAIT fails fast
// with ErrBusy. Without row locks (caps.Locking false) building ForUpdate is
// ErrUnsupported.
func LockingNowaitSkipLocked(t *testing.T, db *momo.DB, caps Caps) {
	CreateDocuments(t, db, caps)
	ctx := context.Background()
	seedDocs(t, db, [2]any{1, "a"}, [2]any{1, "b"})

	q := momo.Select[Document](db).Where(DocTenantID.Eq(1)).OrderBy(DocTitle.Asc())
	if !caps.Locking {
		if _, err := q.ForUpdate("SKIP LOCKED").SQL(); !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("ForUpdate without row locks: err = %v, want ErrUnsupported", err)
		}
		return
	}

	holder, err := db.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Rollback(ctx) }()
	held, err := momo.Select[Document](holder).Where(DocTitle.Eq("a")).ForUpdate().All(ctx)
	if err != nil || len(held) != 1 {
		t.Fatalf("holder lock = %v, %v; want one row", held, err)
	}

	skipper, err := db.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = skipper.Rollback(ctx) }()
	rows, err := momo.Select[Document](skipper).Where(DocTenantID.Eq(1)).OrderBy(DocTitle.Asc()).ForUpdate("SKIP LOCKED").All(ctx)
	if err != nil || len(rows) != 1 || rows[0].Title != "b" {
		t.Fatalf("SKIP LOCKED = %+v, %v; want only the unlocked row b", rows, err)
	}

	nowait, err := db.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = nowait.Rollback(ctx) }()
	if _, err := momo.Select[Document](nowait).Where(DocTitle.Eq("a")).ForUpdate("NOWAIT").All(ctx); !errors.Is(err, momo.ErrBusy) {
		t.Fatalf("NOWAIT on a held row: err = %v, want ErrBusy", err)
	}

	// Once the holder lets go, the row is available again.
	if err := holder.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	again, err := db.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = again.Rollback(ctx) }()
	rows, err = momo.Select[Document](again).Where(DocTitle.Eq("a")).ForUpdate("NOWAIT").All(ctx)
	if err != nil || len(rows) != 1 {
		t.Fatalf("NOWAIT after release = %v, %v; want one row", rows, err)
	}
}

// WithTableNameQuoted asserts that WithTableName quotes the name (it is never
// spliced) and that the replacement table is the one that is read.
func WithTableNameQuoted(t *testing.T, db *momo.DB, caps Caps) {
	CreateDocuments(t, db, caps)
	ctx := context.Background()
	seedDocs(t, db, [2]any{1, "in-documents"})

	for _, name := range []string{`documents"; DROP TABLE documents; --`, `x"y`} {
		c, err := momo.Select[Document](db).WithTableName(name).SQL()
		if err != nil {
			t.Fatal(err)
		}
		if want := db.Dialect().QuoteIdent(name); !strings.Contains(c.SQL, want) {
			t.Fatalf("table name %q not quoted: %s", name, c.SQL)
		}
	}

	if _, err := momo.Exec(ctx, db, `DROP TABLE IF EXISTS docs_alt`); err != nil {
		t.Fatal(err)
	}
	ddl := documentsDDLPostgres
	switch caps.Dialect {
	case dialect.SQLite:
		ddl = documentsDDLSQLite
	case dialect.MySQL:
		ddl = documentsDDLMySQL
	}
	if _, err := momo.Exec(ctx, db, strings.Replace(ddl, "CREATE TABLE documents", "CREATE TABLE docs_alt", 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := momo.Exec(ctx, db, "INSERT INTO docs_alt (tenant_id, title) VALUES (?, ?)", 1, "in-alt"); err != nil {
		t.Fatal(err)
	}
	rows, err := momo.Select[Document](db).Where(DocTenantID.Eq(1)).WithTableName("docs_alt").All(ctx)
	if err != nil || len(rows) != 1 || rows[0].Title != "in-alt" {
		t.Fatalf("WithTableName read = %+v, %v; want the docs_alt row", rows, err)
	}
	rows, err = momo.Select[Document](db).Where(DocTenantID.Eq(1)).All(ctx)
	if err != nil || len(rows) != 1 || rows[0].Title != "in-documents" {
		t.Fatalf("base query = %+v, %v; want the documents row", rows, err)
	}
}

// HintsByDialect asserts IndexedBy: SQLite renders and runs INDEXED BY with a
// quoted name, every other dialect returns ErrUnsupported when building.
func HintsByDialect(t *testing.T, db *momo.DB, caps Caps) {
	CreateDocuments(t, db, caps)
	ctx := context.Background()
	seedDocs(t, db, [2]any{1, "x"})

	c, err := momo.Select[Document](db).IndexedBy(`idx"x`).SQL()
	if caps.Dialect != dialect.SQLite {
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("%s IndexedBy err = %v, want ErrUnsupported", caps.Dialect, err)
		}
		return
	}
	if err != nil || !strings.Contains(c.SQL, `INDEXED BY "idx""x"`) {
		t.Fatalf("sqlite IndexedBy: %v %s", err, c.SQL)
	}
	if _, err := momo.Exec(ctx, db, `CREATE INDEX docs_title_idx ON documents (title)`); err != nil {
		t.Fatal(err)
	}
	rows, err := momo.Select[Document](db).IndexedBy("docs_title_idx").Where(DocTitle.Eq("x")).All(ctx)
	if err != nil || len(rows) != 1 {
		t.Fatalf("INDEXED BY run = %v, %v; want one row", rows, err)
	}
}
