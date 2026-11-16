// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/schema"
	"github.com/azrtydxb/momo/vector"
)

// hdoc is the fixture of the portable hybrid tests: a text column covered by a
// full-text declaration, a nullable two-dimensional embedding and a
// soft-delete marker.
type hdoc struct {
	ID   int64
	Body string
	Emb  *vector.Vector
	Gone *time.Time
}

var hdocMeta = momo.Meta[hdoc]{
	Table:      "hdocs",
	Columns:    []momo.ColumnMeta{{Name: "id"}, {Name: "body"}, {Name: "emb"}, {Name: "deleted_at"}},
	Ptr:        func(m *hdoc, i int) any { return []any{&m.ID, &m.Body, &m.Emb, &m.Gone}[i] },
	Value:      func(m *hdoc, i int) any { return []any{m.ID, m.Body, m.Emb, m.Gone}[i] },
	Identity:   -1,
	Version:    -1,
	SoftDelete: 3,
	CreatedAt:  -1,
	UpdatedAt:  -1,
	PK:         []int{0},
	TableSchema: &schema.Table{
		Name:       "hdocs",
		PrimaryKey: []string{"id"},
		Indexes: []schema.Index{{Name: "hdocs_search", Method: "fulltext",
			Parts:    []schema.IndexPart{{Column: "body"}},
			FullText: &schema.FullText{Columns: []string{"body"}}}},
	},
}

func (d *hdoc) MomoMeta() *momo.Meta[hdoc] { return &hdocMeta }

var (
	hdBody = momo.NewColumn[string]("hdocs", "body", 0)
	hdEmb  = momo.NewVectorColumn[vector.Vector]("hdocs", "emb", 2, 0)
)

// hybridCapable reports whether the server can run the hybrid statement:
// SQLite with sqlite-vec, or MariaDB.
func hybridCapable(db *momo.DB, d dialect.Name) bool {
	switch d {
	case dialect.SQLite:
		return true
	case dialect.MySQL:
		return db.Dialect().Flavor == dialect.FlavorMariaDB
	}
	return false
}

func hybridExec(t *testing.T, db *momo.DB, stmts ...string) {
	t.Helper()
	for _, s := range stmts {
		if _, err := db.Driver().Exec(context.Background(), s, nil); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

// createHybridTable creates the hdocs table with the search structure of its
// declaration: external-content FTS5 and its triggers on SQLite, a FULLTEXT
// index on MariaDB.
func createHybridTable(t *testing.T, db *momo.DB, d dialect.Name) {
	t.Helper()
	switch d {
	case dialect.SQLite:
		typ, err := momo.SQLiteVectorType("emb", 2)
		if err != nil {
			t.Fatal(err)
		}
		hybridExec(t, db,
			`CREATE TABLE hdocs (id INTEGER PRIMARY KEY, body TEXT NOT NULL, emb `+typ+`, deleted_at TIMESTAMP NULL)`,
			`CREATE VIRTUAL TABLE hdocs_fts USING fts5(body, content='hdocs', content_rowid='id')`,
			`CREATE TRIGGER hdocs_ai AFTER INSERT ON hdocs BEGIN
				INSERT INTO hdocs_fts(rowid, body) VALUES (new.id, new.body);
			END`,
			`CREATE TRIGGER hdocs_ad AFTER DELETE ON hdocs BEGIN
				INSERT INTO hdocs_fts(hdocs_fts, rowid, body) VALUES ('delete', old.id, old.body);
			END`,
			`CREATE TRIGGER hdocs_au AFTER UPDATE ON hdocs BEGIN
				INSERT INTO hdocs_fts(hdocs_fts, rowid, body) VALUES ('delete', old.id, old.body);
				INSERT INTO hdocs_fts(rowid, body) VALUES (new.id, new.body);
			END`)
	case dialect.MySQL:
		hybridExec(t, db,
			"CREATE TABLE hdocs (id BIGINT PRIMARY KEY, body TEXT NOT NULL, emb VECTOR(2), deleted_at DATETIME NULL, FULLTEXT KEY hdocs_search (body)) ENGINE=InnoDB")
	default:
		t.Fatalf("dialect %s is not covered here", d)
	}
}

func hv(x, y float32) *vector.Vector {
	v := vector.Vector{x, y}
	return &v
}

// hybridSeed is the fixed corpus. Documents 1 and 2 match "fox" in text, and
// document 2 ranks first there (three occurrences in a three-word body). The
// vectors lie at distinct angles from the query vector (1, 0): document 2 is
// the nearest, then 3, then 1, then 4 to 9; document 10 has no embedding.
func hybridSeed() []hdoc {
	return []hdoc{
		{ID: 1, Body: "the quick brown fox jumps over the lazy dog", Emb: hv(1, 2)},
		{ID: 2, Body: "fox fox fox", Emb: hv(1, 0)},
		{ID: 3, Body: "a slow green turtle", Emb: hv(1, 1)},
		{ID: 4, Body: "apple banana cherry", Emb: hv(1, 3)},
		{ID: 5, Body: "delta epsilon zeta", Emb: hv(1, 4)},
		{ID: 6, Body: "granite basalt marble", Emb: hv(1, 5)},
		{ID: 7, Body: "river lake ocean", Emb: hv(1, 6)},
		{ID: 8, Body: "winter summer autumn", Emb: hv(1, 7)},
		{ID: 9, Body: "copper silver gold", Emb: hv(1, 8)},
		{ID: 10, Body: "nullvec", Emb: nil},
	}
}

func insertHybrid(t *testing.T, db *momo.DB, rows []hdoc) {
	t.Helper()
	if _, err := momo.Insert[hdoc](db).Models(rows).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func seedHybridDocs(t *testing.T, db *momo.DB, d dialect.Name) {
	t.Helper()
	createHybridTable(t, db, d)
	insertHybrid(t, db, hybridSeed())
}

func hybridRun(t *testing.T, db *momo.DB, query string, vecCandidates int) []momo.HybridResult[hdoc] {
	t.Helper()
	res, err := momo.HybridSearch[hdoc](db, momo.HybridSpec{
		Text: hdBody, TextConfig: "english", Query: query, Vector: hdEmb,
		QueryVector: vector.Vector{1, 0}, VectorCandidates: vecCandidates, RRFK: 60, Limit: 20,
	}).All(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func hybridIDsOf(res []momo.HybridResult[hdoc]) []int64 {
	ids := make([]int64, len(res))
	for i, r := range res {
		ids[i] = r.Row.ID
	}
	return ids
}

// Catches: the RRF order or any score differing from the sum of 1/(RRFK+rank)
// computed from each leg's own ranking, and a row in one leg outranking a row
// in both.
func TestHybridSearchFusionSQLiteMariaDB(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if !hybridCapable(db, d) {
			t.Skip("hybrid needs SQLite with sqlite-vec or MariaDB 11.8")
		}
		seedHybridDocs(t, db, d)
		const k = 60
		res := hybridRun(t, db, "fox", 2)
		textRank := map[int64]int{2: 1, 1: 2} // bm25 / MATCH order of the seed
		vecRank := map[int64]int{2: 1, 3: 2}  // cosine order of the seed, two candidates
		want := map[int64]float64{}
		for id, r := range textRank {
			want[id] += 1.0 / float64(k+r)
		}
		for id, r := range vecRank {
			want[id] += 1.0 / float64(k+r)
		}
		if len(res) != len(want) {
			t.Fatalf("rows = %v, want %d rows", hybridIDsOf(res), len(want))
		}
		if res[0].Row.ID != 2 {
			t.Fatalf("the row in both legs is not first: %v", hybridIDsOf(res))
		}
		for _, r := range res {
			if math.Abs(r.Score-want[r.Row.ID]) > 1e-9 {
				t.Errorf("id %d: score %v, want %v", r.Row.ID, r.Score, want[r.Row.ID])
			}
		}
	})
}

// Catches: the order depending on insertion order when rows tie in a leg and in
// the fused score.
func TestHybridSearchTieBreakPortable(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if !hybridCapable(db, d) {
			t.Skip("hybrid needs SQLite with sqlite-vec or MariaDB 11.8")
		}
		createHybridTable(t, db, d)
		rows := []hdoc{
			{ID: 1, Body: "fox", Emb: hv(1, 1)},
			{ID: 2, Body: "fox", Emb: hv(1, 1)},
			{ID: 3, Body: "fox", Emb: hv(1, 1)},
			{ID: 4, Body: "unrelated words here", Emb: hv(-1, 1)},
		}
		insertHybrid(t, db, rows)
		first := hybridRun(t, db, "fox", 0)
		hybridExec(t, db, "DELETE FROM hdocs")
		insertHybrid(t, db, []hdoc{rows[3], rows[2], rows[0], rows[1]})
		second := hybridRun(t, db, "fox", 0)
		if want := []int64{1, 2, 3, 4}; !slices.Equal(hybridIDsOf(first), want) || !slices.Equal(hybridIDsOf(second), want) {
			t.Fatalf("order = %v and %v, want %v both times", hybridIDsOf(first), hybridIDsOf(second), want)
		}
		for i := range first {
			if first[i].Score != second[i].Score {
				t.Errorf("row %d: score %v then %v", first[i].Row.ID, first[i].Score, second[i].Score)
			}
		}
	})
}

// Catches: a query that matches no text returning nothing, an empty text leg
// by construction sending a MATCH, and a row with a NULL embedding coming back
// through the vector leg.
func TestHybridSearchEmptyTextLegPortable(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if !hybridCapable(db, d) {
			t.Skip("hybrid needs SQLite with sqlite-vec or MariaDB 11.8")
		}
		seedHybridDocs(t, db, d)
		for _, q := range []string{"zzzz", "-fox"} { // matches nothing, and no positive word
			res := hybridRun(t, db, q, 2)
			if got, want := hybridIDsOf(res), []int64{2, 3}; !slices.Equal(got, want) {
				t.Fatalf("%q: ids = %v, want %v", q, got, want)
			}
			for i, r := range res {
				if want := 1.0 / float64(60+i+1); math.Abs(r.Score-want) > 1e-9 {
					t.Errorf("%q: id %d score %v, want %v", q, r.Row.ID, r.Score, want)
				}
			}
		}
		all := hybridRun(t, db, "zzzz", 100)
		if len(all) != 9 || slices.Contains(hybridIDsOf(all), 10) {
			t.Fatalf("ids = %v, want the nine rows with an embedding and not 10", hybridIDsOf(all))
		}
	})
}

// Catches: a soft-deleted row returned through the text leg or the vector leg.
func TestHybridSearchSoftDeletePortable(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if !hybridCapable(db, d) {
			t.Skip("hybrid needs SQLite with sqlite-vec or MariaDB 11.8")
		}
		seedHybridDocs(t, db, d)
		// Document 1 is in the text leg only, document 3 in the vector leg only.
		hybridExec(t, db, "UPDATE hdocs SET deleted_at = CURRENT_TIMESTAMP WHERE id IN (1, 3)")
		res := hybridRun(t, db, "fox", 2)
		ids := hybridIDsOf(res)
		if slices.Contains(ids, 1) || slices.Contains(ids, 3) {
			t.Fatalf("a soft-deleted row was returned: %v", ids)
		}
		if want := []int64{2, 4}; !slices.Equal(ids, want) {
			t.Fatalf("ids = %v, want %v", ids, want)
		}
	})
}

// Catches: MySQL accepting a hybrid search it cannot answer (no vector
// distance function) instead of refusing it.
func TestHybridSearchMySQLRefusedLive(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		if db.Dialect().Flavor == dialect.FlavorMariaDB {
			t.Skip("MariaDB runs hybrid search")
		}
		_, err := momo.HybridSearch[hdoc](db, momo.HybridSpec{
			Text: hdBody, Query: "fox", Vector: hdEmb, QueryVector: vector.Vector{1, 0}, Limit: 5,
		}).All(context.Background())
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("err = %v, want ErrUnsupported", err)
		}
	})
}
