// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"errors"
	"math"
	"math/bits"
	"slices"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/internal/vectorparity"
	"github.com/azrtydxb/momo/vecop"
	"github.com/azrtydxb/momo/vector"
)

// These tests run the typed vector builders against SQLite with sqlite-vec.
// They open the database with testopen.SQLite and not testopen.Each: a vector
// column needs the sqlite-vec DDL (a BLOB with a length CHECK), which is not
// the DDL of the other dialects.

type sqvBitsDoc struct {
	ID   int64
	Bits vector.Bits
}

var sqvBitsDocMeta = momo.Meta[sqvBitsDoc]{
	Table:      "bdocs",
	Columns:    []momo.ColumnMeta{{Name: "id"}, {Name: "bits"}},
	Ptr:        func(m *sqvBitsDoc, i int) any { return []any{&m.ID, &m.Bits}[i] },
	Value:      func(m *sqvBitsDoc, i int) any { return []any{m.ID, m.Bits}[i] },
	Identity:   -1,
	Version:    -1,
	SoftDelete: -1,
	CreatedAt:  -1,
	UpdatedAt:  -1,
	PK:         []int{0},
}

func (d *sqvBitsDoc) MomoMeta() *momo.Meta[sqvBitsDoc] { return &sqvBitsDocMeta }

// sqvNullDoc has the nullable vector field type, *vector.Vector.
type sqvNullDoc struct {
	ID  int64
	Emb *vector.Vector
}

var sqvNullDocMeta = momo.Meta[sqvNullDoc]{
	Table:      "ndocs",
	Columns:    []momo.ColumnMeta{{Name: "id"}, {Name: "emb"}},
	Ptr:        func(m *sqvNullDoc, i int) any { return []any{&m.ID, &m.Emb}[i] },
	Value:      func(m *sqvNullDoc, i int) any { return []any{m.ID, m.Emb}[i] },
	Identity:   -1,
	Version:    -1,
	SoftDelete: -1,
	CreatedAt:  -1,
	UpdatedAt:  -1,
	PK:         []int{0},
}

func (d *sqvNullDoc) MomoMeta() *momo.Meta[sqvNullDoc] { return &sqvNullDocMeta }

// sqvSecretDoc has a sensitive embedding.
type sqvSecretDoc struct {
	ID  int64
	Emb vector.Vector
}

var sqvSecretDocMeta = momo.Meta[sqvSecretDoc]{
	Table:      "sdocs",
	Columns:    []momo.ColumnMeta{{Name: "id"}, {Name: "emb", Flags: momo.FlagSensitive}},
	Ptr:        func(m *sqvSecretDoc, i int) any { return []any{&m.ID, &m.Emb}[i] },
	Value:      func(m *sqvSecretDoc, i int) any { return []any{m.ID, m.Emb}[i] },
	Identity:   -1,
	Version:    -1,
	SoftDelete: -1,
	CreatedAt:  -1,
	UpdatedAt:  -1,
	PK:         []int{0},
}

func (d *sqvSecretDoc) MomoMeta() *momo.Meta[sqvSecretDoc] { return &sqvSecretDocMeta }

// sqvDistances runs a compiled "id, distance" select and returns the distance
// of each id.
func sqvDistances(t *testing.T, db *momo.DB, c momo.Compiled) map[int64]float64 {
	t.Helper()
	rows, err := db.Driver().Query(t.Context(), c.SQL, c.Args)
	if err != nil {
		t.Fatalf("query %q: %v", c.SQL, err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Error(err)
		}
	}()
	got := map[int64]float64{}
	for rows.Next() {
		var id int64
		var d float64
		if err := rows.Scan(&id, &d); err != nil {
			t.Fatal(err)
		}
		got[id] = d
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return got
}

func sqvIDs(docs []vecDoc) []int64 {
	ids := make([]int64, len(docs))
	for i, d := range docs {
		ids[i] = d.ID
	}
	return ids
}

// sqvDocsTable creates docs(id, emb vector(3), radius) and loads three rows.
func sqvDocsTable(t *testing.T, db *momo.DB) []vector.Vector {
	t.Helper()
	typ, err := momo.SQLiteVectorType("emb", 3)
	if err != nil {
		t.Fatal(err)
	}
	mustExecPG(t, db, "CREATE TABLE docs (id INTEGER PRIMARY KEY, emb "+typ+" NOT NULL, radius REAL NOT NULL DEFAULT 5)")
	rows := []vector.Vector{{1, 2, 3}, {4, 5, 6}, {10, 10, 10}}
	models := make([]vecDoc, len(rows))
	for i, v := range rows {
		models[i] = vecDoc{ID: int64(i + 1), Emb: v}
	}
	if _, err := momo.Insert[vecDoc](db).Models(models).Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
	return rows
}

// TestSQLiteVectorDistances checks the value of every dense metric against the
// plain-Go reference, and that the distances order, filter and project.
func TestSQLiteVectorDistances(t *testing.T) {
	db := testopen.SQLite(t)
	rows := sqvDocsTable(t, db)
	col := momo.NewVectorColumn[vector.Vector]("docs", "emb", 3, 0)
	idCol := momo.ColumnOf[int64]("docs", "id")
	q := vector.Vector{4, 5, 6}
	cases := []struct {
		name string
		expr vecop.Expr
		ref  func(a, b vector.Vector) float64
	}{
		{"L2Distance", vecop.L2(col, q), vectorparity.L2Distance},
		{"L1Distance", vecop.L1(col, q), vectorparity.L1Distance},
		{"CosineDistance", vecop.Cosine(col, q), vectorparity.CosineDistance},
	}
	for _, tc := range cases {
		c, err := momo.Select[vecDoc, *vecDoc](db).Columns(idCol, tc.expr).SQL()
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		got := sqvDistances(t, db, c)
		if len(got) != len(rows) {
			t.Fatalf("%s: got %d rows, want %d", tc.name, len(got), len(rows))
		}
		for i, v := range rows {
			id, want := int64(i+1), tc.ref(v, q)
			if g, ok := got[id]; !ok || math.Abs(g-want) > 1e-5 {
				t.Errorf("%s id %d = %v, want %v (1e-5)", tc.name, id, g, want)
			}
		}
	}

	// Nearest first, farthest first, and the typed read back of the vectors.
	near, err := momo.Select[vecDoc](db).OrderBy(vecop.L2(col, q).Asc()).All(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := sqvIDs(near), []int64{2, 1, 3}; !slices.Equal(got, want) {
		t.Fatalf("nearest first = %v, want %v", got, want)
	}
	for _, d := range near {
		if !slices.Equal(d.Emb, rows[d.ID-1]) {
			t.Errorf("id %d reads back as %v, want %v", d.ID, d.Emb, rows[d.ID-1])
		}
	}
	far, err := momo.Select[vecDoc](db).OrderBy(vecop.L2(col, q).Desc()).All(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := sqvIDs(far), []int64{3, 1, 2}; !slices.Equal(got, want) {
		t.Fatalf("farthest first = %v, want %v", got, want)
	}

	// A distance in a WHERE clause: radius 5 holds only the query itself (0),
	// the next row is sqrt(27), about 5.2.
	radius := momo.NewColumn[float64]("docs", "radius", 0)
	within, err := momo.Select[vecDoc](db).Where(radius.GtExpr(vecop.L2(col, q))).All(t.Context())
	if err != nil || len(within) != 1 || within[0].ID != 2 {
		t.Fatalf("within radius: %v, %v, want only id 2", within, err)
	}

	// Equality on the vector binds the blob.
	eq, err := momo.Select[vecDoc](db).Where(col.Eq(vector.Vector{10, 10, 10})).All(t.Context())
	if err != nil || len(eq) != 1 || eq[0].ID != 3 {
		t.Fatalf("equality: %v, %v, want only id 3", eq, err)
	}
}

// TestSQLiteBitVectorHamming covers the bit vector type: stored as the packed
// bytes, compared through vec_bit.
func TestSQLiteBitVectorHamming(t *testing.T) {
	db := testopen.SQLite(t)
	typ, err := momo.SQLiteBitsType("bits", 8)
	if err != nil {
		t.Fatal(err)
	}
	mustExecPG(t, db, "CREATE TABLE bdocs (id INTEGER PRIMARY KEY, bits "+typ+" NOT NULL)")
	data := []byte{0xB0, 0x98, 0xFF}
	models := make([]sqvBitsDoc, len(data))
	for i, b := range data {
		models[i] = sqvBitsDoc{ID: int64(i + 1), Bits: vector.Bits{Len: 8, Data: []byte{b}}}
	}
	if _, err := momo.Insert[sqvBitsDoc](db).Models(models).Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
	col := momo.NewVectorColumn[vector.Bits]("bdocs", "bits", 8, 0)
	q := vector.Bits{Len: 8, Data: []byte{0xB0}}
	c, err := momo.Select[sqvBitsDoc](db).Columns(momo.ColumnOf[int64]("bdocs", "id"), vecop.Hamming(col, q)).SQL()
	if err != nil {
		t.Fatal(err)
	}
	got := sqvDistances(t, db, c)
	for i, b := range data {
		if want := float64(bits.OnesCount8(b ^ 0xB0)); got[int64(i+1)] != want {
			t.Errorf("hamming id %d = %v, want %v", i+1, got[int64(i+1)], want)
		}
	}
	if got[2] != 2 {
		t.Errorf("hamming(0xB0, 0x98) = %v, want 2 (the popcount of 0x28)", got[2])
	}

	ordered, err := momo.Select[sqvBitsDoc](db).OrderBy(vecop.Hamming(col, q).Asc()).All(t.Context())
	if err != nil || len(ordered) != 3 {
		t.Fatalf("ordered: %v, %v", ordered, err)
	}
	for i, want := range []int64{1, 2, 3} {
		if ordered[i].ID != want {
			t.Fatalf("hamming order = %v, want ids 1 2 3", ordered)
		}
		if ordered[i].Bits.Len != 8 || !slices.Equal(ordered[i].Bits.Data, []byte{data[want-1]}) {
			t.Fatalf("id %d reads back as %+v, want the byte %#x", want, ordered[i].Bits, data[want-1])
		}
	}
	// Jaccard has no sqlite-vec function.
	_, err = momo.Select[sqvBitsDoc](db).OrderBy(vecop.Jaccard(col, q).Asc()).All(t.Context())
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("jaccard: err = %v, want ErrUnsupported", err)
	}
}

// TestSQLiteVectorNullable inserts, updates and reads the nullable field type.
func TestSQLiteVectorNullable(t *testing.T) {
	db := testopen.SQLite(t)
	typ, err := momo.SQLiteVectorType("emb", 3)
	if err != nil {
		t.Fatal(err)
	}
	mustExecPG(t, db, "CREATE TABLE ndocs (id INTEGER PRIMARY KEY, emb "+typ+")")
	first := vector.Vector{1, 2, 3}
	if _, err := momo.Insert[sqvNullDoc](db).Models([]sqvNullDoc{{ID: 1, Emb: &first}, {ID: 2}}).Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
	col := momo.NewNullVectorColumn[vector.Vector]("ndocs", "emb", 3, 0)
	id := momo.NewColumn[int64]("ndocs", "id", 0)
	if _, err := momo.Update[sqvNullDoc](db).Set(col.Set(vector.Vector{4, 5, 6})).Where(id.Eq(2)).Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
	got, err := momo.Select[sqvNullDoc](db).Where(col.Eq(vector.Vector{4, 5, 6})).All(t.Context())
	if err != nil || len(got) != 1 || got[0].ID != 2 || got[0].Emb == nil || !slices.Equal(*got[0].Emb, vector.Vector{4, 5, 6}) {
		t.Fatalf("after update: %v, %v", got, err)
	}
	one, err := momo.Select[sqvNullDoc](db).Where(id.Eq(1)).All(t.Context())
	if err != nil || len(one) != 1 || one[0].Emb == nil || !slices.Equal(*one[0].Emb, first) {
		t.Fatalf("non-nil pointer insert: %v, %v", one, err)
	}
	if _, err := momo.Update[sqvNullDoc](db).Set(col.SetNull()).Where(id.Eq(1)).Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
	nulls, err := momo.Select[sqvNullDoc](db).Where(col.IsNull()).All(t.Context())
	if err != nil || len(nulls) != 1 || nulls[0].ID != 1 || nulls[0].Emb != nil {
		t.Fatalf("after SetNull: %v, %v, want id 1 with a nil vector", nulls, err)
	}
}

// TestSQLiteVectorRefusals: what sqlite-vec cannot do is ErrUnsupported, what
// the value or the column forbids is a validation or check error, and none of
// it reaches the database as a silently different query.
func TestSQLiteVectorRefusals(t *testing.T) {
	db := testopen.SQLite(t)
	sqvDocsTable(t, db)
	col := momo.NewVectorColumn[vector.Vector]("docs", "emb", 3, 0)
	q := vector.Vector{1, 2, 3}
	ctx := t.Context()

	unsupported := map[string]momo.Order{
		"inner product": vecop.InnerProduct(col, q).Asc(),
		"halfvec":       vecop.L2(momo.NewVectorColumn[vector.HalfVector]("docs", "emb", 3, 0), vector.HalfVector{1, 2, 3}).Asc(),
		"sparsevec":     vecop.L2(momo.NewVectorColumn[vector.SparseVector]("docs", "emb", 3, 0), vector.SparseVector{Dim: 3, Indices: []int32{0}, Values: []float32{1}}).Asc(),
	}
	for name, order := range unsupported {
		if _, err := momo.Select[vecDoc](db).OrderBy(order).All(ctx); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s: err = %v, want ErrUnsupported", name, err)
		}
	}

	if _, err := momo.Select[vecDoc](db).OrderBy(vecop.L2(col, vector.Vector{1, 2}).Asc()).All(ctx); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("dimension mismatch: err = %v, want ErrValidation", err)
	}
	if _, err := momo.Select[vecDoc](db).OrderBy(vecop.L2(col, vector.Vector{1, float32(math.NaN()), 3}).Asc()).All(ctx); !errors.Is(err, momo.ErrInvalidVector) {
		t.Errorf("NaN query: err = %v, want ErrInvalidVector", err)
	}

	// A vector of the wrong size is kept out by the length CHECK, and a value
	// that does not validate never reaches the table.
	if _, err := momo.Insert[vecDoc](db).Models([]vecDoc{{ID: 10, Emb: vector.Vector{1, 2}}}).Exec(ctx); !errors.Is(err, momo.ErrCheckViolation) {
		t.Errorf("two elements in a vector(3) column: err = %v, want ErrCheckViolation", err)
	}
	if _, err := momo.Insert[vecDoc](db).Models([]vecDoc{{ID: 11, Emb: vector.Vector{1, float32(math.Inf(1)), 3}}}).Exec(ctx); !errors.Is(err, momo.ErrInvalidVector) {
		t.Errorf("Inf element: err = %v, want ErrInvalidVector", err)
	}
	left, err := momo.Select[vecDoc](db).All(ctx)
	if err != nil || len(left) != 3 {
		t.Fatalf("rows after refused inserts: %d, %v, want the original 3", len(left), err)
	}

	// There is no approximate index to ask for.
	if idx := db.Driver().Dialect().Vector.Indexes; len(idx) != 0 {
		t.Errorf("the SQLite dialect lists vector indexes %v", idx)
	}

	// Search settings: every SQLite search is exact, so ExactSearch is a no-op
	// and a setting that tunes an index is refused.
	if err := db.WithSearch(ctx, momo.SearchSettings{ExactSearch: true}, func(tx *momo.Tx) error {
		_, err := momo.Select[vecDoc](tx).OrderBy(vecop.L2(col, q).Asc()).Limit(1).All(ctx)
		return err
	}); err != nil {
		t.Errorf("ExactSearch: %v", err)
	}
	err = db.WithSearch(ctx, momo.SearchSettings{HNSWEfSearch: 40}, func(*momo.Tx) error { return nil })
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("HNSWEfSearch: err = %v, want ErrUnsupported", err)
	}
}

// TestSQLiteVectorSensitiveRedaction: a sensitive embedding shows as
// RedactedArg in hook events on every bind path (insert, query vector) while
// the database receives and returns the real vector.
func TestSQLiteVectorSensitiveRedaction(t *testing.T) {
	rec := &obstest.Recorder{}
	db := testopen.SQLite(t, momo.WithHook(rec), momo.WithArgLogging())
	typ, err := momo.SQLiteVectorType("emb", 3)
	if err != nil {
		t.Fatal(err)
	}
	mustExecPG(t, db, "CREATE TABLE sdocs (id INTEGER PRIMARY KEY, emb "+typ+" NOT NULL)")
	secret := vector.Vector{7, 8, 9}
	if _, err := momo.Insert[sqvSecretDoc](db).Models([]sqvSecretDoc{{ID: 1, Emb: secret}}).Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
	col := momo.NewVectorColumn[vector.Vector]("sdocs", "emb", 3, momo.FlagSensitive)
	got, err := momo.Select[sqvSecretDoc](db).OrderBy(vecop.Cosine(col, secret).Asc()).All(t.Context())
	if err != nil || len(got) != 1 || !slices.Equal(got[0].Emb, secret) {
		t.Fatalf("round trip: %v, %v", got, err)
	}
	seen := map[string]bool{}
	for _, e := range rec.Events() {
		if e.Op != "insert" && e.Op != "select" {
			continue
		}
		seen[e.Op] = true
		if len(e.Args) == 0 || e.Args[len(e.Args)-1] != momo.RedactedArg {
			t.Errorf("%s: hook saw args %v, want the vector as RedactedArg", e.Op, e.Args)
		}
		for _, a := range e.Args {
			if _, isBlob := a.([]byte); isBlob {
				t.Errorf("%s: hook saw the raw vector blob", e.Op)
			}
		}
	}
	if !seen["insert"] || !seen["select"] {
		t.Fatalf("recorded ops %v, want an insert and a select", seen)
	}
}
