// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/azrtydxb/momo/schema"
)

// unnestKind has a nullable text, a timestamptz (plain and nullable), a bytea
// and a numeric column, the types whose array binding differs from int and text.
type unnestKind struct {
	ID    int64
	Note  *string
	At    time.Time
	Maybe *time.Time
	Blob  []byte
	Amt   float64
}

func (r *unnestKind) MomoMeta() *momo.Meta[unnestKind] { return &unnestKindMeta }

var (
	unnestKindID  = momo.NewColumn[int64]("unnest_kinds", "id", 0)
	unnestKindAmt = momo.NewColumn[float64]("unnest_kinds", "amt", 0)
)

var unnestKindMeta = momo.Meta[unnestKind]{
	Table: "unnest_kinds",
	PK:    []int{0},
	Columns: []momo.ColumnMeta{
		{Name: "id"}, {Name: "note"}, {Name: "at"}, {Name: "maybe"}, {Name: "blob"}, {Name: "amt"},
	},
	Ptr: func(r *unnestKind, i int) any {
		return [...]any{&r.ID, &r.Note, &r.At, &r.Maybe, &r.Blob, &r.Amt}[i]
	},
	Value: func(r *unnestKind, i int) any {
		return [...]any{r.ID, r.Note, r.At, r.Maybe, r.Blob, r.Amt}[i]
	},
	TableSchema: &schema.Table{
		Name: "unnest_kinds",
		Columns: []schema.Column{
			{Name: "id", Type: "bigint"},
			{Name: "note", Type: "text"},
			{Name: "at", Type: "timestamptz"},
			{Name: "maybe", Type: "timestamptz"},
			{Name: "blob", Type: "bytea"},
			{Name: "amt", Type: "numeric"},
		},
		PrimaryKey: []string{"id"},
	},
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func unnestKindsDB(t *testing.T) (*momo.DB, *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	const ddl = `CREATE TABLE unnest_kinds (
		id bigint PRIMARY KEY, note text, at timestamptz NOT NULL, maybe timestamptz,
		blob bytea NOT NULL, amt numeric NOT NULL)`
	if _, err := pool.Exec(ctx, ddl); err != nil {
		t.Fatal(err)
	}
	return momo.New(pgxdriver.New(pool)), pool
}

func unnestKindRows() []unnestKind {
	note := "hello"
	maybe := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	return []unnestKind{
		{ID: 1, Note: &note, At: time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC), Maybe: &maybe, Blob: []byte{0, 1, 2}, Amt: 1.5},
		{ID: 2, Note: nil, At: time.Date(2026, 6, 7, 8, 9, 10, 0, time.UTC), Maybe: nil, Blob: []byte{}, Amt: 2.25},
		{ID: 3, Note: &note, At: time.Date(2026, 7, 8, 9, 10, 11, 0, time.UTC), Maybe: nil, Blob: []byte("x"), Amt: 100},
	}
}

func readUnnestKinds(t *testing.T, db *momo.DB) []unnestKind {
	t.Helper()
	got, err := momo.Select[unnestKind](db).OrderBy(unnestKindID.Asc()).All(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for i := range got {
		got[i].At = got[i].At.UTC()
		if got[i].Maybe != nil {
			u := got[i].Maybe.UTC()
			got[i].Maybe = &u
		}
	}
	return got
}

// TestUnnestNullableTimestampBytesNumeric inserts NULLs, timestamptz, bytea and
// numeric values through one unnest statement and reads them back.
func TestUnnestNullableTimestampBytesNumeric(t *testing.T) {
	db, _ := unnestKindsDB(t)
	rows := unnestKindRows()
	res, err := momo.Insert[unnestKind](db).Models(rows).Unnest().Exec(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.RowsAffected != int64(len(rows)) {
		t.Fatalf("affected %d, want %d", res.RowsAffected, len(rows))
	}
	got := readUnnestKinds(t, db)
	if len(got) != len(rows) {
		t.Fatalf("read %d rows, want %d", len(got), len(rows))
	}
	for i, w := range rows {
		g := got[i]
		if g.ID != w.ID || g.Amt != w.Amt || !g.At.Equal(w.At) || string(g.Blob) != string(w.Blob) {
			t.Errorf("row %d = %+v, want %+v", i, g, w)
		}
		if !reflect.DeepEqual(g.Note, w.Note) {
			t.Errorf("row %d note = %v, want %v", i, g.Note, w.Note)
		}
		if (g.Maybe == nil) != (w.Maybe == nil) || (w.Maybe != nil && !g.Maybe.Equal(*w.Maybe)) {
			t.Errorf("row %d maybe = %v, want %v", i, g.Maybe, w.Maybe)
		}
	}
}

// TestUnnestOnConflictAndReturning checks that Unnest carries ON CONFLICT and
// RETURNING: existing rows are updated from the proposed values, and the
// statement returns one row per inserted or updated row.
func TestUnnestOnConflictAndReturning(t *testing.T) {
	db, _ := unnestKindsDB(t)
	ctx := context.Background()
	rows := unnestKindRows()
	if _, err := momo.Insert[unnestKind](db).Models(rows[:2]).Unnest().Exec(ctx); err != nil {
		t.Fatal(err)
	}

	skip, err := momo.Insert[unnestKind](db).Models(rows).Unnest().OnConflict(unnestKindID).DoNothing().Exec(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if skip.RowsAffected != 1 {
		t.Fatalf("DoNothing affected %d rows, want only the new row", skip.RowsAffected)
	}

	changed := unnestKindRows()
	changed[0].Amt = 99.5
	changed[1].Amt = 98.5
	cur, err := momo.Insert[unnestKind](db).Models(changed).Unnest().
		OnConflict(unnestKindID).DoUpdate(unnestKindAmt.SetExcluded()).
		Returning(unnestKindID, unnestKindAmt).Rows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var amts []float64
	for cur.Next() {
		var r unnestKind
		if err := cur.ScanModel(&r); err != nil {
			t.Fatal(err)
		}
		amts = append(amts, r.Amt)
	}
	if err := cur.Close(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(amts, []float64{99.5, 98.5, 100}) {
		t.Fatalf("returned amounts = %v", amts)
	}
	got := readUnnestKinds(t, db)
	if got[0].Amt != 99.5 || got[1].Amt != 98.5 || got[2].Amt != 100 {
		t.Fatalf("stored amounts = %v %v %v", got[0].Amt, got[1].Amt, got[2].Amt)
	}
}

// TestUnnestRefusesValuesInsert checks that Unnest says so for a Values insert.
func TestUnnestRefusesValuesInsert(t *testing.T) {
	db, _ := unnestKindsDB(t)
	_, err := momo.Insert[unnestKind](db).Values(unnestKindAmt.Set(1)).Unnest().Exec(context.Background())
	if !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}
