// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// arrRow reads an id, a title and a text[] column.
type arrRow struct {
	ID    int64
	Title string
	Tags  []string
}

var arrRowMeta = momo.Meta[arrRow]{
	Columns:  []momo.ColumnMeta{{Name: "id"}, {Name: "title"}, {Name: "tags"}},
	Ptr:      func(m *arrRow, i int) any { return [...]any{&m.ID, &m.Title, &m.Tags}[i] },
	Value:    func(m *arrRow, i int) any { return [...]any{m.ID, m.Title, m.Tags}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*arrRow) MomoMeta() *momo.Meta[arrRow] { return &arrRowMeta }

var (
	arrLiveID    = momo.NewColumn[int64]("arr_docs", "id", momo.FlagPK)
	arrLiveTitle = momo.NewColumn[string]("arr_docs", "title", 0)
	arrLiveTags  = momo.NewColumn[[]string]("arr_docs", "tags", 0)
)

// Catches: a wrong array operator, a slice bound as anything but one array
// parameter, an empty or nil slice bound as NULL, and a text[] value that does
// not scan back into []string.
func TestArrayOperatorsLive(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	if _, err := momo.Exec(ctx, db, "CREATE TABLE arr_docs (id bigint PRIMARY KEY, title text NOT NULL, tags text[] NOT NULL DEFAULT '{}')"); err != nil {
		t.Fatal(err)
	}
	seed := []arrRow{
		{1, "j", []string{"a", "b"}},
		{2, "k", []string{"c"}},
		{3, "l", []string{"a", "b", "c"}},
		{4, "m", []string{}},
	}
	for _, r := range seed {
		if _, err := momo.Exec(ctx, db, "INSERT INTO arr_docs (id, title, tags) VALUES (?, ?, ?)", r.ID, r.Title, r.Tags); err != nil {
			t.Fatal(err)
		}
	}
	ids := func(p momo.Predicate) []int64 {
		t.Helper()
		rows, err := momo.Select[arrRow](db).From(momo.TableRef{Name: "arr_docs"}).
			Columns(arrLiveID, arrLiveTitle, arrLiveTags).Where(p).OrderBy(arrLiveID.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		out := []int64{}
		for _, r := range rows {
			out = append(out, r.ID)
		}
		return out
	}
	cases := []struct {
		name string
		pred momo.Predicate
		want []int64
	}{
		{"contains a,b", momo.ArrayContains(arrLiveTags, "a", "b"), []int64{1, 3}},
		{"contains nothing", momo.ArrayContains[string](arrLiveTags), []int64{1, 2, 3, 4}},
		{"contained by a,b", momo.ArrayContainedBy(arrLiveTags, "a", "b"), []int64{1, 4}},
		{"contained by nothing", momo.ArrayContainedBy[string](arrLiveTags), []int64{4}},
		{"overlaps b,z", momo.ArrayOverlaps(arrLiveTags, "b", "z"), []int64{1, 3}},
		{"overlaps nothing", momo.ArrayOverlaps[string](arrLiveTags), []int64{}},
		{"title any of j,l", momo.AnyOf(arrLiveTitle, "j", "l"), []int64{1, 3}},
		{"title any of nothing", momo.AnyOf[string](arrLiveTitle), []int64{}},
		{"combined", momo.And(momo.ArrayOverlaps(arrLiveTags, "c"), momo.Not(momo.AnyOf(arrLiveTitle, "k"))), []int64{3}},
	}
	for _, c := range cases {
		if got := ids(c.pred); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: ids = %v, want %v", c.name, got, c.want)
		}
	}
	rows, err := momo.Select[arrRow](db).From(momo.TableRef{Name: "arr_docs"}).
		Columns(arrLiveID, arrLiveTitle, arrLiveTags).Where(arrLiveID.Eq(3)).All(ctx)
	if err != nil || len(rows) != 1 || !reflect.DeepEqual(rows[0].Tags, []string{"a", "b", "c"}) {
		t.Fatalf("scan text[] = %+v, %v", rows, err)
	}
}

// Catches: a malformed element list reaching the database as SQL: values are
// bound, so a hostile element is just a string.
func TestArrayOperatorsBindHostileValues(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	if _, err := momo.Exec(ctx, db, "CREATE TABLE arr_docs (id bigint PRIMARY KEY, title text NOT NULL, tags text[] NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	evil := `x'); DROP TABLE arr_docs; --`
	if _, err := momo.Exec(ctx, db, "INSERT INTO arr_docs VALUES (1, 'j', ?)", []string{evil, `a,b`, `"q"`}); err != nil {
		t.Fatal(err)
	}
	n, err := momo.Select[arrRow](db).From(momo.TableRef{Name: "arr_docs"}).
		Columns(arrLiveID).Where(momo.ArrayContains(arrLiveTags, evil, "a,b", `"q"`)).Count(ctx)
	if err != nil || n != 1 {
		t.Fatalf("count = %d, %v", n, err)
	}
	left, err := momo.Scalars[int64](ctx, db, "SELECT count(*) FROM arr_docs")
	if err != nil || len(left) != 1 || left[0] != 1 {
		t.Fatalf("table damaged: %v, %v", left, err)
	}
}
