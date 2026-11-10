// SPDX-License-Identifier: Apache-2.0

//go:build integration

package arrayop_test

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/arrayop"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/types"
)

// kvRow is a two-field projection: a key and a count.
type kvRow struct {
	K int64
	N int64
}

var kvRowMeta = momo.Meta[kvRow]{
	Columns:  []momo.ColumnMeta{{Name: "k"}, {Name: "n"}},
	Ptr:      func(m *kvRow, i int) any { return [...]any{&m.K, &m.N}[i] },
	Value:    func(m *kvRow, i int) any { return [...]any{m.K, m.N}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*kvRow) MomoMeta() *momo.Meta[kvRow] { return &kvRowMeta }

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
		{"contains a,b", arrayop.Contains(arrLiveTags, "a", "b"), []int64{1, 3}},
		{"contains nothing", arrayop.Contains[string](arrLiveTags), []int64{1, 2, 3, 4}},
		{"contained by a,b", arrayop.ContainedBy(arrLiveTags, "a", "b"), []int64{1, 4}},
		{"contained by nothing", arrayop.ContainedBy[string](arrLiveTags), []int64{4}},
		{"overlaps b,z", arrayop.Overlaps(arrLiveTags, "b", "z"), []int64{1, 3}},
		{"overlaps nothing", arrayop.Overlaps[string](arrLiveTags), []int64{}},
		{"title any of j,l", arrayop.AnyOf(arrLiveTitle, "j", "l"), []int64{1, 3}},
		{"title any of nothing", arrayop.AnyOf[string](arrLiveTitle), []int64{}},
		{"combined", momo.And(arrayop.Overlaps(arrLiveTags, "c"), momo.Not(arrayop.AnyOf(arrLiveTitle, "k"))), []int64{3}},
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
		Columns(arrLiveID, arrLiveTitle, arrLiveTags).Where(arrayop.Contains(arrLiveTags, evil, "a,b", `"q"`)).Count(ctx)
	if err != nil || n != 1 {
		t.Fatalf("count = %d, %v", n, err)
	}
	left, err := momo.Scalars[int64](ctx, db, "SELECT count(*) FROM arr_docs")
	if err != nil || len(left) != 1 || left[0] != 1 {
		t.Fatalf("table damaged: %v, %v", left, err)
	}
}

// Catches: element types other than text failing to bind as one typed array
// (smallint[] and bigint[] columns), including values at the type limits.
func TestArrayContainsOnIntegerColumns(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	if _, err := momo.Exec(ctx, db, "CREATE TABLE arr_ints (id bigint PRIMARY KEY, s smallint[] NOT NULL, b bigint[] NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	if _, err := momo.Exec(ctx, db, "INSERT INTO arr_ints VALUES (1, ?, ?), (2, ?, ?)",
		[]int16{math.MinInt16, 7}, []int64{math.MaxInt64, 1},
		[]int16{8}, []int64{2}); err != nil {
		t.Fatal(err)
	}
	id := momo.NewColumn[int64]("arr_ints", "id", momo.FlagPK)
	small := momo.NewColumn[[]int16]("arr_ints", "s", 0)
	big := momo.NewColumn[[]int64]("arr_ints", "b", 0)
	ids := func(p momo.Predicate) []int64 {
		t.Helper()
		rows, err := momo.Select[kvRow](db).From(momo.TableRef{Name: "arr_ints"}).
			Columns(id, id).Where(p).OrderBy(id.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		out := []int64{}
		for _, r := range rows {
			out = append(out, r.K)
		}
		return out
	}
	for name, c := range map[string]struct {
		pred momo.Predicate
		want []int64
	}{
		"int16 contains":     {arrayop.Contains(small, int16(math.MinInt16), 7), []int64{1}},
		"int16 overlaps":     {arrayop.Overlaps(small, 7, 8), []int64{1, 2}},
		"int64 contains":     {arrayop.Contains(big, int64(math.MaxInt64)), []int64{1}},
		"int64 contained by": {arrayop.ContainedBy(big, 2, 3), []int64{2}},
	} {
		if got := ids(c.pred); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: ids = %v, want %v", name, got, c.want)
		}
	}
}

// Catches: a nil slice silently stored as an empty array. It binds as NULL,
// so a NOT NULL array column rejects it; docs/TYPE_MAPPING.md says so.
func TestNilSliceIntoNotNullArrayColumn(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	if _, err := momo.Exec(ctx, db, "CREATE TABLE arr_nn (id bigint PRIMARY KEY, tags text[] NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	var nilTags []string
	if _, err := momo.Exec(ctx, db, "INSERT INTO arr_nn VALUES (1, ?)", nilTags); !errors.Is(err, momo.ErrNotNullViolation) {
		t.Fatalf("nil slice err = %v, want ErrNotNullViolation", err)
	}
	if _, err := momo.Exec(ctx, db, "INSERT INTO arr_nn VALUES (2, ?)", []string{}); err != nil {
		t.Fatalf("empty slice: %v", err)
	}
}

// uidRow reads an id and a uuid[] column.
type uidRow struct {
	ID   int64
	Uids []types.UUID
}

var uidRowMeta = momo.Meta[uidRow]{
	Columns:  []momo.ColumnMeta{{Name: "id"}, {Name: "uids"}},
	Ptr:      func(m *uidRow, i int) any { return [...]any{&m.ID, &m.Uids}[i] },
	Value:    func(m *uidRow, i int) any { return [...]any{m.ID, m.Uids}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*uidRow) MomoMeta() *momo.Meta[uidRow] { return &uidRowMeta }

// Catches: a []types.UUID that does not bind as one uuid[] parameter, a uuid
// array predicate with the wrong operator, a uuid[] value that does not scan
// back, and a uuid column that = ANY cannot match.
func TestArrayOperatorsOnUUIDLive(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	if _, err := momo.Exec(ctx, db, "CREATE TABLE arr_uids (id bigint PRIMARY KEY, uid uuid NOT NULL, uids uuid[] NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	a, b, c := types.UUID{1}, types.UUID{2}, types.UUID{3}
	seed := []struct {
		id   int64
		uid  types.UUID
		uids []types.UUID
	}{
		{1, a, []types.UUID{a, b}},
		{2, b, []types.UUID{c}},
		{3, c, []types.UUID{a, b, c}},
		{4, a, []types.UUID{}},
	}
	for _, r := range seed {
		if _, err := momo.Exec(ctx, db, "INSERT INTO arr_uids VALUES (?, ?, ?)", r.id, r.uid, r.uids); err != nil {
			t.Fatal(err)
		}
	}
	id := momo.NewColumn[int64]("arr_uids", "id", momo.FlagPK)
	uid := momo.NewColumn[types.UUID]("arr_uids", "uid", 0)
	uids := momo.NewColumn[[]types.UUID]("arr_uids", "uids", 0)
	run := func(p momo.Predicate) []uidRow {
		t.Helper()
		rows, err := momo.Select[uidRow](db).From(momo.TableRef{Name: "arr_uids"}).
			Columns(id, uids).Where(p).OrderBy(id.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	ids := func(p momo.Predicate) []int64 {
		out := []int64{}
		for _, r := range run(p) {
			out = append(out, r.ID)
		}
		return out
	}
	cases := []struct {
		name string
		pred momo.Predicate
		want []int64
	}{
		{"contains a,b", arrayop.Contains(uids, a, b), []int64{1, 3}},
		{"contains nothing", arrayop.Contains[types.UUID](uids), []int64{1, 2, 3, 4}},
		{"contained by a,b", arrayop.ContainedBy(uids, a, b), []int64{1, 4}},
		{"overlaps c", arrayop.Overlaps(uids, c), []int64{2, 3}},
		{"overlaps nothing", arrayop.Overlaps[types.UUID](uids), []int64{}},
		{"uid any of b,c", arrayop.AnyOf(uid, b, c), []int64{2, 3}},
		{"uid any of nothing", arrayop.AnyOf[types.UUID](uid), []int64{}},
	}
	for _, tc := range cases {
		if got := ids(tc.pred); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: ids = %v, want %v", tc.name, got, tc.want)
		}
	}
	if rows := run(id.Eq(1)); len(rows) != 1 || !reflect.DeepEqual(rows[0].Uids, []types.UUID{a, b}) {
		t.Fatalf("scan uuid[] = %+v", rows)
	}
}
