// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/internal/vectortype"
	"github.com/azrtydxb/momo/vector"
)

// embDoc is a model with an embedding kept in sync with its title and body.
// The hash column emb_hash is not a field: the generator adds it to the table,
// so the Meta names it in Embeddings.
type embDoc struct {
	ID    int64
	Title string
	Body  *string
	Emb   *vector.Vector
}

var embDocMeta = momo.Meta[embDoc]{
	Table:      "embed_docs",
	Columns:    []momo.ColumnMeta{{Name: "id"}, {Name: "title"}, {Name: "body"}, {Name: "emb"}},
	Ptr:        func(m *embDoc, i int) any { return []any{&m.ID, &m.Title, &m.Body, &m.Emb}[i] },
	Value:      func(m *embDoc, i int) any { return []any{m.ID, m.Title, m.Body, m.Emb}[i] },
	Identity:   -1,
	Version:    -1,
	SoftDelete: -1,
	CreatedAt:  -1,
	UpdatedAt:  -1,
	PK:         []int{0},
	Embeddings: []momo.EmbeddingMeta{{Vector: 3, Dim: 3, HashColumn: "emb_hash", Sources: []int{1, 2}}},
}

func (d *embDoc) MomoMeta() *momo.Meta[embDoc] { return &embDocMeta }

// embFake is an Embedder that records its batches. Its vector for a text is
// {len(text), 1, 0}, so a test can tell which text a row was embedded from.
type embFake struct {
	mu      sync.Mutex
	batches [][]string
	// during runs inside Embed, after the batch is recorded.
	during func(texts []string)
}

func (f *embFake) Embed(_ context.Context, texts []string) ([]vector.Vector, error) {
	f.mu.Lock()
	f.batches = append(f.batches, slices.Clone(texts))
	during := f.during
	f.mu.Unlock()
	if during != nil {
		during(texts)
	}
	out := make([]vector.Vector, len(texts))
	for i, s := range texts {
		out[i] = vector.Vector{float32(len(s)), 1, 0}
	}
	return out, nil
}

func (f *embFake) calls() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.batches)
}

func embCreate(t *testing.T, db *momo.DB, d dialect.Name) {
	t.Helper()
	vec := "vector(3)"
	if d == dialect.SQLite {
		typ, err := vectortype.SQLite("emb", 3)
		if err != nil {
			t.Fatal(err)
		}
		vec = typ
	} else {
		mustExecPG(t, db, "CREATE EXTENSION IF NOT EXISTS vector")
	}
	mustExecPG(t, db, "CREATE TABLE embed_docs (id bigint PRIMARY KEY, title text NOT NULL, body text, emb "+vec+", emb_hash varchar(64), tenant_id bigint NOT NULL DEFAULT 1)")
}

func embInsert(t *testing.T, db *momo.DB, id int64, title string, body *string) {
	t.Helper()
	mustExecPG(t, db, "INSERT INTO embed_docs (id, title, body) VALUES (?, ?, ?)", id, title, body)
}

func embStr(s string) *string { return &s }

// embHashes returns the stored source hash of every row.
func embHashes(t *testing.T, db *momo.DB) map[int64]sql.NullString {
	t.Helper()
	rows, err := momo.QueryCursor(t.Context(), db, "SELECT id, emb_hash FROM embed_docs")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Error(err)
		}
	}()
	out := map[int64]sql.NullString{}
	for rows.Next() {
		var id int64
		var h sql.NullString
		if err := rows.Scan(&id, &h); err != nil {
			t.Fatal(err)
		}
		out[id] = h
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// embVectors returns the stored embedding of every row; nil is SQL NULL.
func embVectors(t *testing.T, db *momo.DB) map[int64]*vector.Vector {
	t.Helper()
	docs, err := momo.Select[embDoc, *embDoc](db).All(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	out := map[int64]*vector.Vector{}
	for _, d := range docs {
		out[d.ID] = d.Emb
	}
	return out
}

func embStale(t *testing.T, db *momo.DB) []int64 {
	t.Helper()
	var ids []int64
	for m, err := range momo.StaleEmbeddings[embDoc](t.Context(), db, momo.EmbedOptions{}) {
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, m.ID)
	}
	return ids
}

func embWant(t *testing.T, got *vector.Vector, want vector.Vector) {
	t.Helper()
	if got == nil || !slices.Equal(*got, want) {
		t.Fatalf("embedding = %v, want %v", got, want)
	}
}

// Catches: a sync that skips a stale row, writes the vector without the hash
// (so the row is picked up again), embeds a row that has no text, ignores the
// batch size, or does work on a run that has nothing to do.
func TestSyncEmbeddingsFillsAndIsIdempotent(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		embCreate(t, db, d)
		embInsert(t, db, 1, "alpha", embStr("one"))
		embInsert(t, db, 2, "beta", nil)
		embInsert(t, db, 3, "", nil)

		if got, want := embStale(t, db), []int64{1, 2, 3}; !slices.Equal(got, want) {
			t.Fatalf("stale before sync = %v, want %v", got, want)
		}
		f := &embFake{}
		res, err := momo.SyncEmbeddings[embDoc](t.Context(), db, f, momo.EmbedOptions{BatchSize: 2})
		if err != nil {
			t.Fatal(err)
		}
		if res.Embedded != 2 || res.Cleared != 1 || res.Conflicts != 0 {
			t.Fatalf("result = %+v, want 2 embedded, 1 cleared, 0 conflicts", res)
		}
		// Row 3 has no text, so it never reaches the Embedder; rows 1 and 2
		// fill the one batch of two.
		if got, want := f.calls(), [][]string{{"alpha\n\none", "beta"}}; !reflect.DeepEqual(got, want) {
			t.Fatalf("embedder batches = %q, want %q", got, want)
		}

		vecs := embVectors(t, db)
		embWant(t, vecs[1], vector.Vector{10, 1, 0})
		embWant(t, vecs[2], vector.Vector{4, 1, 0})
		if vecs[3] != nil {
			t.Fatalf("row without text has embedding %v, want NULL", *vecs[3])
		}
		hashes := embHashes(t, db)
		for id, want := range map[int64]string{
			1: vector.SourceHash(sql.NullString{String: "alpha", Valid: true}, sql.NullString{String: "one", Valid: true}),
			2: vector.SourceHash(sql.NullString{String: "beta", Valid: true}, sql.NullString{}),
			3: vector.SourceHash(sql.NullString{Valid: true}, sql.NullString{}),
		} {
			if !hashes[id].Valid || hashes[id].String != want {
				t.Fatalf("row %d hash = %+v, want %s", id, hashes[id], want)
			}
		}

		// Nothing is stale now, so a second run does no work.
		res, err = momo.SyncEmbeddings[embDoc](t.Context(), db, f, momo.EmbedOptions{BatchSize: 2})
		if err != nil {
			t.Fatal(err)
		}
		if res != (momo.EmbedResult{}) || len(f.calls()) != 1 {
			t.Fatalf("second run = %+v with %d embedder calls, want nothing done", res, len(f.calls()))
		}
	})
}

// Catches: a stale query that misses a changed source, a NULLed vector or a
// cleared source, or reports a row that is current.
func TestStaleEmbeddingsFindsChangedRows(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		embCreate(t, db, d)
		for i := int64(1); i <= 4; i++ {
			embInsert(t, db, i, fmt.Sprintf("title %d", i), embStr("body"))
		}
		if _, err := momo.SyncEmbeddings[embDoc](t.Context(), db, &embFake{}, momo.EmbedOptions{}); err != nil {
			t.Fatal(err)
		}
		if got := embStale(t, db); len(got) != 0 {
			t.Fatalf("stale after sync = %v, want none", got)
		}

		mustExecPG(t, db, "UPDATE embed_docs SET title = ? WHERE id = ?", "renamed", int64(2))
		mustExecPG(t, db, "UPDATE embed_docs SET emb = NULL WHERE id = ?", int64(3))
		mustExecPG(t, db, "UPDATE embed_docs SET body = NULL WHERE id = ?", int64(4))
		if got, want := embStale(t, db), []int64{2, 3, 4}; !slices.Equal(got, want) {
			t.Fatalf("stale = %v, want %v (changed title, NULL vector, cleared body)", got, want)
		}

		res, err := momo.SyncEmbeddings[embDoc](t.Context(), db, &embFake{}, momo.EmbedOptions{Limit: 2})
		if err != nil {
			t.Fatal(err)
		}
		if res.Embedded != 2 {
			t.Fatalf("limited run embedded %d rows, want 2", res.Embedded)
		}
		if got, want := embStale(t, db), []int64{4}; !slices.Equal(got, want) {
			t.Fatalf("stale after a limit-2 run = %v, want %v", got, want)
		}
	})
}

// Catches: the stale read or the write-back ignoring the handle's interceptors,
// so a row outside a tenant scope would be listed as stale or have an
// embedding written by a tenant-scoped runner.
func TestSyncEmbeddingsAppliesInterceptors(t *testing.T) {
	tenant := momo.NewColumn[int64]("embed_docs", "tenant_id", 0)
	scope := momo.WithInterceptor(func(_ context.Context, op *momo.Operation) error {
		op.Where = append(op.Where, tenant.Eq(1))
		return nil
	})
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		embCreate(t, db, d)
		embInsert(t, db, 1, "mine", nil)
		embInsert(t, db, 2, "also mine", nil)
		embInsert(t, db, 3, "theirs", nil)
		mustExecPG(t, db, "UPDATE embed_docs SET tenant_id = 2 WHERE id = ?", int64(3))

		if got, want := embStale(t, db), []int64{1, 2}; !slices.Equal(got, want) {
			t.Fatalf("stale in scope = %v, want %v", got, want)
		}
		res, err := momo.SyncEmbeddings[embDoc](t.Context(), db, &embFake{}, momo.EmbedOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if res.Embedded != 2 {
			t.Fatalf("result = %+v, want 2 embedded", res)
		}
		hashes := embHashes(t, db)
		if !hashes[1].Valid || !hashes[2].Valid || hashes[3].Valid {
			t.Fatalf("hashes = %+v, want rows 1 and 2 written and row 3 untouched", hashes)
		}
	}, scope)
}

// Catches: the keyset page boundary skipping or repeating a row. 300 rows are
// more than one page of 256, so the second page must start right after the
// last row of the first.
func TestSyncEmbeddingsPagesThroughTheTable(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		embCreate(t, db, d)
		const rows = 300
		var b strings.Builder
		var args []any
		b.WriteString("INSERT INTO embed_docs (id, title) VALUES ")
		for i := int64(1); i <= rows; i++ {
			if i > 1 {
				b.WriteString(", ")
			}
			b.WriteString("(?, ?)")
			args = append(args, i, fmt.Sprintf("row %d", i))
		}
		mustExecPG(t, db, b.String(), args...)

		want := make([]int64, rows)
		for i := range want {
			want[i] = int64(i + 1)
		}
		if got := embStale(t, db); !slices.Equal(got, want) {
			t.Fatalf("stale rows = %d ids (first %v), want 1..%d once each, in order", len(got), got[:min(len(got), 5)], rows)
		}
		res, err := momo.SyncEmbeddings[embDoc](t.Context(), db, &embFake{}, momo.EmbedOptions{BatchSize: 50})
		if err != nil {
			t.Fatal(err)
		}
		if res.Embedded != rows || res.Conflicts != 0 {
			t.Fatalf("result = %+v, want %d embedded", res, rows)
		}
		if got := embStale(t, db); len(got) != 0 {
			t.Fatalf("%d rows still stale after the sync", len(got))
		}
	})
}

// Catches: a row whose source changes while the Embedder runs being marked
// fresh with the old text's embedding. The compare-and-set must fail, leave
// the row stale and count it, and the next run must embed the new text.
func TestSyncEmbeddingsLostRaceIsNotMarkedFresh(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		embCreate(t, db, d)
		embInsert(t, db, 1, "before", nil)
		embInsert(t, db, 2, "steady", nil)

		f := &embFake{}
		f.during = func([]string) {
			mustExecPG(t, db, "UPDATE embed_docs SET title = ? WHERE id = ?", "after the edit", int64(1))
		}
		res, err := momo.SyncEmbeddings[embDoc](t.Context(), db, f, momo.EmbedOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if res.Embedded != 1 || res.Conflicts != 1 {
			t.Fatalf("result = %+v, want 1 embedded and 1 conflict", res)
		}
		if h := embHashes(t, db)[1]; h.Valid {
			t.Fatalf("row 1 was marked fresh with hash %q after its source changed mid-embed", h.String)
		}
		if vecs := embVectors(t, db); vecs[1] != nil {
			t.Fatalf("row 1 holds embedding %v of the old text", *vecs[1])
		}
		if got, want := embStale(t, db), []int64{1}; !slices.Equal(got, want) {
			t.Fatalf("stale = %v, want %v", got, want)
		}

		f.during = nil
		res, err = momo.SyncEmbeddings[embDoc](t.Context(), db, f, momo.EmbedOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if res.Embedded != 1 || res.Conflicts != 0 {
			t.Fatalf("resumed result = %+v, want 1 embedded", res)
		}
		embWant(t, embVectors(t, db)[1], vector.Vector{float32(len("after the edit")), 1, 0})
	})
}

// Catches: a row that is stale only because its vector is NULL (its hash is
// current) being written by two runners and counted as embedded twice.
func TestSyncEmbeddingsNullVectorWrittenOnce(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		embCreate(t, db, d)
		embInsert(t, db, 1, "alpha", nil)
		mustExecPG(t, db, "UPDATE embed_docs SET emb_hash = ? WHERE id = ?", vector.SourceHash(sql.NullString{String: "alpha", Valid: true}, sql.NullString{}), int64(1))
		if got, want := embStale(t, db), []int64{1}; !slices.Equal(got, want) {
			t.Fatalf("stale = %v, want %v", got, want)
		}

		var other momo.EmbedResult
		a := &embFake{}
		a.during = func([]string) {
			var err error
			other, err = momo.SyncEmbeddings[embDoc](t.Context(), db, &embFake{}, momo.EmbedOptions{})
			if err != nil {
				t.Error(err)
			}
		}
		res, err := momo.SyncEmbeddings[embDoc](t.Context(), db, a, momo.EmbedOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if other.Embedded != 1 {
			t.Fatalf("the second runner result = %+v, want 1 embedded", other)
		}
		if res.Embedded != 0 || res.Conflicts != 1 {
			t.Fatalf("the first runner result = %+v, want no row written and 1 conflict", res)
		}
	})
}

// Catches: two runners that both write a row (a lost update of the hash), a
// row skipped by both, or a runner that holds a lock during the Embedder
// call and so blocks the other.
func TestSyncEmbeddingsConcurrentRunners(t *testing.T) {
	db := testopen.Postgres(t)
	embCreate(t, db, dialect.Postgres)
	const rows = 40
	for i := int64(1); i <= rows; i++ {
		embInsert(t, db, i, fmt.Sprintf("document %d", i), embStr("text"))
	}
	f := &embFake{}
	var wg sync.WaitGroup
	results := make([]momo.EmbedResult, 2)
	errs := make([]error, 2)
	for r := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[r], errs[r] = momo.SyncEmbeddings[embDoc](context.Background(), db, f, momo.EmbedOptions{BatchSize: 5})
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := results[0].Embedded + results[1].Embedded; got != rows {
		t.Fatalf("runners wrote %d rows between them (%+v), want exactly %d", got, results, rows)
	}
	if got := embStale(t, db); len(got) != 0 {
		t.Fatalf("stale after both runners = %v, want none", got)
	}
	vecs := embVectors(t, db)
	for i := int64(1); i <= rows; i++ {
		embWant(t, vecs[i], vector.Vector{float32(len(fmt.Sprintf("document %d\n\ntext", i))), 1, 0})
	}
}

// Catches: an Embedder that returns the wrong number of vectors writing
// anything, and an Embedder failure being swallowed.
func TestSyncEmbeddingsEmbedderFailures(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		embCreate(t, db, d)
		embInsert(t, db, 1, "one", nil)

		_, err := momo.SyncEmbeddings[embDoc](t.Context(), db, shortEmbedder{}, momo.EmbedOptions{})
		if !errors.Is(err, momo.ErrValidation) {
			t.Fatalf("short embedder error = %v, want ErrValidation", err)
		}
		boom := errors.New("provider down")
		_, err = momo.SyncEmbeddings[embDoc](t.Context(), db, failEmbedder{boom}, momo.EmbedOptions{})
		if !errors.Is(err, boom) {
			t.Fatalf("failing embedder error = %v, want it to wrap %v", err, boom)
		}
		if got, want := embStale(t, db), []int64{1}; !slices.Equal(got, want) {
			t.Fatalf("stale after failed runs = %v, want %v", got, want)
		}
	})
}

type shortEmbedder struct{}

func (shortEmbedder) Embed(context.Context, []string) ([]vector.Vector, error) { return nil, nil }

type failEmbedder struct{ err error }

func (f failEmbedder) Embed(context.Context, []string) ([]vector.Vector, error) { return nil, f.err }
