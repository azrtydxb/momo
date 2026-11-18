// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/vector"
)

type embuDoc struct {
	ID    int64
	Title string
	Body  *string
	Emb   *vector.Vector
}

var embuDocMeta = momo.Meta[embuDoc]{
	Table:      "docs",
	Columns:    []momo.ColumnMeta{{Name: "id"}, {Name: "title"}, {Name: "body"}, {Name: "emb"}},
	Ptr:        func(m *embuDoc, i int) any { return []any{&m.ID, &m.Title, &m.Body, &m.Emb}[i] },
	Value:      func(m *embuDoc, i int) any { return []any{m.ID, m.Title, m.Body, m.Emb}[i] },
	Identity:   -1,
	Version:    -1,
	SoftDelete: -1,
	CreatedAt:  -1,
	UpdatedAt:  -1,
	PK:         []int{0},
	Embeddings: []momo.EmbeddingMeta{{Vector: 3, Dim: 3, HashColumn: "emb_hash", Sources: []int{1, 2}}},
}

func (*embuDoc) MomoMeta() *momo.Meta[embuDoc] { return &embuDocMeta }

// embuPlain has no embed_from field.
type embuPlain struct{ ID int64 }

var embuPlainMeta = momo.Meta[embuPlain]{
	Table:      "plain",
	Columns:    []momo.ColumnMeta{{Name: "id"}},
	Ptr:        func(m *embuPlain, i int) any { return []any{&m.ID}[i] },
	Value:      func(m *embuPlain, i int) any { return []any{m.ID}[i] },
	Identity:   -1,
	Version:    -1,
	SoftDelete: -1,
	CreatedAt:  -1,
	UpdatedAt:  -1,
	PK:         []int{0},
}

func (*embuPlain) MomoMeta() *momo.Meta[embuPlain] { return &embuPlainMeta }

// embuPair has a composite key and a soft-delete column.
type embuPair struct {
	A, B    int64
	Title   string
	Emb     *vector.Vector
	Deleted *int64
}

var embuPairMeta = momo.Meta[embuPair]{
	Table: "pairs",
	Columns: []momo.ColumnMeta{{Name: "a"}, {Name: "b"}, {Name: "title"}, {Name: "emb"},
		{Name: "deleted", Flags: momo.FlagSoftDelete | momo.FlagNullable}},
	Ptr:            func(m *embuPair, i int) any { return []any{&m.A, &m.B, &m.Title, &m.Emb, &m.Deleted}[i] },
	Value:          func(m *embuPair, i int) any { return []any{m.A, m.B, m.Title, m.Emb, m.Deleted}[i] },
	Identity:       -1,
	Version:        -1,
	SoftDelete:     4,
	SoftDeleteKind: momo.SoftDeleteUnix,
	CreatedAt:      -1,
	UpdatedAt:      -1,
	PK:             []int{0, 1},
	Embeddings:     []momo.EmbeddingMeta{{Vector: 3, HashColumn: "emb_hash", Sources: []int{2}}},
}

func (*embuPair) MomoMeta() *momo.Meta[embuPair] { return &embuPairMeta }

// embuFixed embeds every text as {1, 2, 3} and records its calls.
type embuFixed struct{ calls [][]string }

func (f *embuFixed) Embed(_ context.Context, texts []string) ([]vector.Vector, error) {
	f.calls = append(f.calls, slices.Clone(texts))
	out := make([]vector.Vector, len(texts))
	for i := range out {
		out[i] = vector.Vector{1, 2, 3}
	}
	return out, nil
}

func ns(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }

// Catches: MySQL and MariaDB being served, which would run statements their
// vector columns cannot take, instead of a clear ErrUnsupported.
func TestSyncEmbeddingsRefusesMySQL(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.MySQL))
	_, err := momo.SyncEmbeddings[embuDoc](t.Context(), db, &embuFixed{}, momo.EmbedOptions{})
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("SyncEmbeddings error = %v, want ErrUnsupported", err)
	}
	for _, err := range momo.StaleEmbeddings[embuDoc](t.Context(), db, momo.EmbedOptions{}) {
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("StaleEmbeddings error = %v, want ErrUnsupported", err)
		}
	}
}

// Catches: a model without an embedding, an unknown column, a negative batch
// or a nil Embedder reaching the database.
func TestSyncEmbeddingsValidation(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv)
	ctx := t.Context()
	if _, err := momo.SyncEmbeddings[embuPlain](ctx, db, &embuFixed{}, momo.EmbedOptions{}); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("model without embed_from: %v, want ErrValidation", err)
	}
	if _, err := momo.SyncEmbeddings[embuDoc](ctx, db, &embuFixed{}, momo.EmbedOptions{Vector: "nope"}); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("unknown vector column: %v, want ErrValidation", err)
	}
	if _, err := momo.SyncEmbeddings[embuDoc](ctx, db, &embuFixed{}, momo.EmbedOptions{BatchSize: -1}); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("negative batch: %v, want ErrValidation", err)
	}
	if _, err := momo.SyncEmbeddings[embuDoc](ctx, db, nil, momo.EmbedOptions{}); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("nil embedder: %v, want ErrValidation", err)
	}
	if n := len(drv.Calls()); n != 0 {
		t.Fatalf("%d statements ran before validation passed", n)
	}
}

// Catches: the page statement reading the wrong columns or order, a fresh row
// being embedded, a stale row being skipped, the embedding written without its
// compare-and-set guards, and a lost compare-and-set being counted as written.
func TestSyncEmbeddingsStatements(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	fresh := vector.SourceHash(ns("beta"), sql.NullString{})
	drv.SetRows([][]any{
		{int64(1), "alpha", "one", nil, int64(1)},
		{int64(2), "beta", nil, ns(fresh), int64(0)},
	})
	f := &embuFixed{}
	res, err := momo.SyncEmbeddings[embuDoc](t.Context(), momo.New(drv), f, momo.EmbedOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// The fake reports zero affected rows, so the one write is a lost race.
	if res != (momo.EmbedResult{Conflicts: 1}) {
		t.Fatalf("result = %+v, want one conflict", res)
	}
	if want := [][]string{{"alpha\n\none"}}; !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("embedder calls = %q, want %q", f.calls, want)
	}
	calls := drv.Calls()
	if len(calls) != 2 {
		t.Fatalf("%d statements, want a page and one update: %v", len(calls), calls)
	}
	if want := `SELECT "id", "title", "body", "emb_hash", CASE WHEN "emb" IS NULL THEN 1 ELSE 0 END FROM "docs" ORDER BY "id" LIMIT 256`; calls[0].SQL != want {
		t.Fatalf("page statement:\n%s\nwant:\n%s", calls[0].SQL, want)
	}
	up := calls[1]
	for _, want := range []string{
		`UPDATE "docs" SET "emb" = $1, "emb_hash" = $2 WHERE "docs"."id" = $3`,
		`"docs"."title" IS NOT DISTINCT FROM $4`,
		`"docs"."body" IS NOT DISTINCT FROM $5`,
		`"docs"."emb_hash" IS NOT DISTINCT FROM $6`,
	} {
		if !strings.Contains(up.SQL, want) {
			t.Fatalf("update statement lacks %q:\n%s", want, up.SQL)
		}
	}
	if len(up.Args) != 6 || up.Args[1] != vector.SourceHash(ns("alpha"), ns("one")) {
		t.Fatalf("update args = %v, want the hash of the text read as the second", up.Args)
	}
}

// Catches: the page or write statement leaving out the predicate an
// interceptor adds, which would read and write rows outside a tenant scope.
func TestSyncEmbeddingsStatementsCarryInterceptorPredicates(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{int64(1), "alpha", "one", nil, int64(1)}})
	tenant := momo.NewColumn[int64]("docs", "tenant_id", 0)
	db := momo.New(drv, momo.WithInterceptor(func(_ context.Context, op *momo.Operation) error {
		op.Where = append(op.Where, tenant.Eq(7))
		return nil
	}))
	if _, err := momo.SyncEmbeddings[embuDoc](t.Context(), db, &embuFixed{}, momo.EmbedOptions{}); err != nil {
		t.Fatal(err)
	}
	calls := drv.Calls()
	if len(calls) != 2 {
		t.Fatalf("%d statements, want a page and one update: %v", len(calls), calls)
	}
	if want := `FROM "docs" WHERE "docs"."tenant_id" = $1 ORDER BY "id" LIMIT 256`; !strings.Contains(calls[0].SQL, want) {
		t.Fatalf("page statement lacks the scope:\n%s", calls[0].SQL)
	}
	if want := `AND "docs"."tenant_id" = $7`; !strings.HasSuffix(calls[1].SQL, want) {
		t.Fatalf("update statement does not end with the scope %q:\n%s", want, calls[1].SQL)
	}
	denied := momo.New(fakedriver.New(dialect.Postgres), momo.WithInterceptor(func(context.Context, *momo.Operation) error {
		return errors.New("no")
	}))
	if _, err := momo.SyncEmbeddings[embuDoc](t.Context(), denied, &embuFixed{}, momo.EmbedOptions{}); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("denied run error = %v, want ErrValidation", err)
	}
}

// Catches: a second page that restarts from the top or skips rows, a composite
// key compared as one column, a soft-deleted row being visited, and a Limit
// that is not honored across pages.
func TestStaleEmbeddingsKeysetAndSoftDelete(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	rows := make([][]any, 256)
	for i := range rows {
		rows[i] = []any{int64(7), int64(9), "t", nil, int64(1)}
	}
	drv.SetRows(rows)
	n := 0
	for m, err := range momo.StaleEmbeddings[embuPair](t.Context(), momo.New(drv), momo.EmbedOptions{Limit: 300}) {
		if err != nil {
			t.Fatal(err)
		}
		if m.A != 7 || m.B != 9 || m.Title != "t" {
			t.Fatalf("row = %+v, want the key and source filled", m)
		}
		n++
	}
	if n != 300 {
		t.Fatalf("yielded %d rows, want the limit of 300", n)
	}
	calls := drv.Calls()
	if len(calls) != 2 {
		t.Fatalf("%d page statements, want 2", len(calls))
	}
	first := `SELECT "a", "b", "title", "emb_hash", CASE WHEN "emb" IS NULL THEN 1 ELSE 0 END FROM "pairs" WHERE "pairs"."deleted" IS NULL ORDER BY "a", "b" LIMIT 256`
	if calls[0].SQL != first {
		t.Fatalf("first page:\n%s\nwant:\n%s", calls[0].SQL, first)
	}
	second := `SELECT "a", "b", "title", "emb_hash", CASE WHEN "emb" IS NULL THEN 1 ELSE 0 END FROM "pairs" WHERE "pairs"."deleted" IS NULL AND (("a" > $1) OR ("a" = $2 AND "b" > $3)) ORDER BY "a", "b" LIMIT 256`
	if calls[1].SQL != second {
		t.Fatalf("second page:\n%s\nwant:\n%s", calls[1].SQL, second)
	}
	if want := []any{int64(7), int64(7), int64(9)}; !reflect.DeepEqual(calls[1].Args, want) {
		t.Fatalf("second page args = %v, want %v", calls[1].Args, want)
	}
}
