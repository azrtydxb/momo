// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/vector"
)

// hybridDoc is hand-written: the hybrid builder needs the table, the columns,
// the primary key and the field pointers.
type hybridDoc struct {
	ID   int64
	Body string
	Emb  vector.Vector
}

var hybridDocMeta = momo.Meta[hybridDoc]{
	Table:      "docs",
	Columns:    []momo.ColumnMeta{{Name: "id"}, {Name: "body"}, {Name: "emb"}},
	Ptr:        func(m *hybridDoc, i int) any { return []any{&m.ID, &m.Body, &m.Emb}[i] },
	Value:      func(m *hybridDoc, i int) any { return []any{m.ID, m.Body, m.Emb}[i] },
	Identity:   -1,
	Version:    -1,
	SoftDelete: -1,
	CreatedAt:  -1,
	UpdatedAt:  -1,
	PK:         []int{0},
}

func (d *hybridDoc) MomoMeta() *momo.Meta[hybridDoc] { return &hybridDocMeta }

func hybridSpec() momo.HybridSpec[hybridDoc] {
	return momo.HybridSpec[hybridDoc]{
		Text:        momo.NewColumn[string]("docs", "body", 0),
		TextConfig:  "english",
		Query:       "vector databases",
		Vector:      momo.NewVectorColumn[vector.Vector]("docs", "emb", 3, 0),
		QueryVector: vector.Vector{1, 0, 0},
		Limit:       10,
	}
}

// Catches: the fused statement losing a leg, the cosine operator, the join
// that fuses the legs or the primary-key tie-break, and the arguments drifting
// from the documented bind order.
func TestHybridSQL(t *testing.T) {
	q := momo.New(fakedriver.New(dialect.Postgres))
	c, err := momo.HybridSearch[hybridDoc](q, hybridSpec()).SQL()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"websearch_to_tsquery($1::regconfig, $2)",
		`t."emb" <=> $4`,
		"FULL OUTER JOIN",
		"LIMIT $3",
		"LIMIT $5",
		"$6::int + a.rnk",
		"ORDER BY f.score DESC, f.pk ASC",
		"LIMIT $7",
		`SELECT t."id", t."body", t."emb", f.score::float8`,
	} {
		if !strings.Contains(c.SQL, want) {
			t.Errorf("SQL lacks %q:\n%s", want, c.SQL)
		}
	}
	if strings.Contains(c.SQL, "IS NULL") {
		t.Errorf("a model without soft delete must not filter on a marker:\n%s", c.SQL)
	}
	// Bind order: text config, query, text candidates, query vector, vector candidates, RRF k, limit.
	wantArgs := []any{"english", "vector databases", 100, vector.Vector{1, 0, 0}, 100, 60, 10}
	if !reflect.DeepEqual(c.Args, wantArgs) {
		t.Fatalf("args = %v, want %v", c.Args, wantArgs)
	}
}

// Catches: the explicit candidate and RRF values not reaching the statement.
func TestHybridSQLExplicitParameters(t *testing.T) {
	q := momo.New(fakedriver.New(dialect.Postgres))
	spec := hybridSpec()
	spec.TextCandidates, spec.VectorCandidates, spec.RRFK = 7, 9, 5
	c, err := momo.HybridSearch[hybridDoc](q, spec).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if c.Args[2] != 7 || c.Args[4] != 9 || c.Args[5] != 5 {
		t.Fatalf("args = %v, want text 7, vector 9, k 5", c.Args)
	}
}

// Catches: a spec outside its documented ranges being sent to the database.
func TestHybridValidation(t *testing.T) {
	q := momo.New(fakedriver.New(dialect.Postgres))
	for name, mut := range map[string]func(*momo.HybridSpec[hybridDoc]){
		"negative RRFK":          func(s *momo.HybridSpec[hybridDoc]) { s.RRFK = -1 },
		"text candidates low":    func(s *momo.HybridSpec[hybridDoc]) { s.TextCandidates = -1 },
		"text candidates high":   func(s *momo.HybridSpec[hybridDoc]) { s.TextCandidates = 1001 },
		"vector candidates high": func(s *momo.HybridSpec[hybridDoc]) { s.VectorCandidates = 1001 },
		"zero limit":             func(s *momo.HybridSpec[hybridDoc]) { s.Limit = 0 },
		"limit high":             func(s *momo.HybridSpec[hybridDoc]) { s.Limit = 1001 },
		"no config":              func(s *momo.HybridSpec[hybridDoc]) { s.TextConfig = "" },
		"wrong dimension":        func(s *momo.HybridSpec[hybridDoc]) { s.QueryVector = vector.Vector{1, 0} },
		"no text column":         func(s *momo.HybridSpec[hybridDoc]) { s.Text = momo.NewColumn[string]("docs", "", 0) },
	} {
		t.Run(name, func(t *testing.T) {
			spec := hybridSpec()
			mut(&spec)
			if _, err := momo.HybridSearch[hybridDoc](q, spec).SQL(); !errors.Is(err, momo.ErrValidation) {
				t.Fatalf("err = %v, want ErrValidation", err)
			}
		})
	}
}

// Catches: hybrid search silently rendering PostgreSQL-only SQL for MySQL or
// SQLite.
func TestHybridUnsupportedDialects(t *testing.T) {
	for _, d := range []dialect.Name{dialect.MySQL, dialect.SQLite} {
		q := momo.New(fakedriver.New(d))
		if _, err := momo.HybridSearch[hybridDoc](q, hybridSpec()).SQL(); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s: err = %v, want ErrUnsupported", d, err)
		}
	}
}

// Catches: All dropping the score column or mis-assigning the scanned fields.
func TestHybridAllScansRowAndScore(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{
		{int64(1), "vector search", vector.Vector{1, 0, 0}, 2.0 / 61},
		{int64(3), "search", vector.Vector{0.5, 0.5, 0}, 1.0 / 62},
	})
	res, err := momo.HybridSearch[hybridDoc](momo.New(drv), hybridSpec()).All(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 || res[0].Row.ID != 1 || res[0].Row.Body != "vector search" || res[1].Row.ID != 3 {
		t.Fatalf("rows = %+v", res)
	}
	if res[0].Score != 2.0/61 || res[1].Score != 1.0/62 {
		t.Fatalf("scores = %v, %v", res[0].Score, res[1].Score)
	}
}

// hybridSoftDoc has a soft-delete marker.
type hybridSoftDoc struct {
	ID   int64
	Body string
	Emb  vector.Vector
	Gone *int64
}

var hybridSoftMeta = momo.Meta[hybridSoftDoc]{
	Table:      "soft_docs",
	Schema:     "kb",
	Columns:    []momo.ColumnMeta{{Name: "id"}, {Name: "body"}, {Name: "emb"}, {Name: "deleted_at"}},
	Identity:   -1,
	Version:    -1,
	SoftDelete: 3,
	CreatedAt:  -1,
	UpdatedAt:  -1,
	PK:         []int{0},
}

func (d *hybridSoftDoc) MomoMeta() *momo.Meta[hybridSoftDoc] { return &hybridSoftMeta }

// Catches: soft-deleted rows reaching either leg of the search, and a
// schema-qualified table losing its schema.
func TestHybridSQLSoftDeleteAndSchema(t *testing.T) {
	q := momo.New(fakedriver.New(dialect.Postgres))
	spec := momo.HybridSpec[hybridSoftDoc]{
		Text:        momo.NewColumn[string]("soft_docs", "body", 0),
		TextConfig:  "simple",
		Vector:      momo.NewVectorColumn[vector.Vector]("soft_docs", "emb", 3, 0),
		QueryVector: vector.Vector{1, 0, 0},
		Limit:       5,
	}
	c, err := momo.HybridSearch[hybridSoftDoc](q, spec).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(c.SQL, `t."deleted_at" IS NULL`); n != 2 {
		t.Errorf("soft-delete filter appears %d times, want 2 (one per leg):\n%s", n, c.SQL)
	}
	if !strings.Contains(c.SQL, `FROM "kb"."soft_docs" t`) {
		t.Errorf("table lost its schema:\n%s", c.SQL)
	}
}
