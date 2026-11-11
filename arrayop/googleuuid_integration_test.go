// SPDX-License-Identifier: Apache-2.0

//go:build integration

package arrayop_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/arrayop"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// guidRow reads a uuid, a nullable uuid and a uuid[] as github.com/google/uuid
// types.
type guidRow struct {
	ID    int64
	UID   uuid.UUID
	Owner *uuid.UUID
	UIDs  []uuid.UUID
}

var guidRowMeta = momo.Meta[guidRow]{
	Columns:  []momo.ColumnMeta{{Name: "id"}, {Name: "uid"}, {Name: "owner"}, {Name: "uids"}},
	Ptr:      func(m *guidRow, i int) any { return [...]any{&m.ID, &m.UID, &m.Owner, &m.UIDs}[i] },
	Value:    func(m *guidRow, i int) any { return [...]any{m.ID, m.UID, m.Owner, m.UIDs}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*guidRow) MomoMeta() *momo.Meta[guidRow] { return &guidRowMeta }

// Catches: a github.com/google/uuid value, pointer or slice that does not bind
// or scan on PostgreSQL, a NULL *uuid.UUID that does not read back as nil, and
// an array predicate over a []uuid.UUID column that errors or matches wrongly.
func TestGoogleUUIDLive(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	if _, err := momo.Exec(ctx, db, "CREATE TABLE guid_rows (id bigint PRIMARY KEY, uid uuid NOT NULL, owner uuid, uids uuid[] NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	a := uuid.MustParse("00000001-0000-0000-0000-000000000000")
	b := uuid.MustParse("00000002-0000-0000-0000-000000000000")
	c := uuid.MustParse("00000003-0000-0000-0000-000000000000")
	seed := []guidRow{
		{1, a, &b, []uuid.UUID{a, b}},
		{2, b, nil, []uuid.UUID{c}},
		{3, c, &a, []uuid.UUID{a, b, c}},
		{4, a, nil, []uuid.UUID{}},
	}
	for _, r := range seed {
		if _, err := momo.Exec(ctx, db, "INSERT INTO guid_rows VALUES (?, ?, ?, ?)", r.ID, r.UID, r.Owner, r.UIDs); err != nil {
			t.Fatal(err)
		}
	}
	id := momo.NewColumn[int64]("guid_rows", "id", momo.FlagPK)
	uid := momo.NewColumn[uuid.UUID]("guid_rows", "uid", 0)
	uids := momo.NewColumn[[]uuid.UUID]("guid_rows", "uids", 0)
	run := func(p momo.Predicate) []guidRow {
		t.Helper()
		rows, err := momo.Select[guidRow](db).From(momo.TableRef{Name: "guid_rows"}).
			Columns(id, uid, momo.NewColumn[*uuid.UUID]("guid_rows", "owner", 0), uids).Where(p).OrderBy(id.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	rows := run(id.Gt(0))
	if len(rows) != len(seed) {
		t.Fatalf("rows = %d, want %d", len(rows), len(seed))
	}
	for i, r := range rows {
		w := seed[i]
		if r.UID != w.UID || !reflect.DeepEqual(r.Owner, w.Owner) || !reflect.DeepEqual(r.UIDs, w.UIDs) {
			t.Errorf("row %d = %+v, want %+v", w.ID, r, w)
		}
	}
	ids := func(p momo.Predicate) []int64 {
		out := []int64{}
		for _, r := range run(p) {
			out = append(out, r.ID)
		}
		return out
	}
	for _, tc := range []struct {
		name string
		pred momo.Predicate
		want []int64
	}{
		{"contains a,b", arrayop.Contains(uids, a, b), []int64{1, 3}},
		{"contained by a,b", arrayop.ContainedBy(uids, a, b), []int64{1, 4}},
		{"overlaps c", arrayop.Overlaps(uids, c), []int64{2, 3}},
		{"overlaps nothing", arrayop.Overlaps[uuid.UUID](uids), []int64{}},
		{"uid any of b,c", arrayop.AnyOf(uid, b, c), []int64{2, 3}},
		{"uid any of nothing", arrayop.AnyOf[uuid.UUID](uid), []int64{}},
		{"uid eq", uid.Eq(a), []int64{1, 4}},
	} {
		if got := ids(tc.pred); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: ids = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// Catches: a uuid.UUID that does not store as canonical text on SQLite or does
// not read back from it, with and without NULL.
func TestGoogleUUIDLiveSQLite(t *testing.T) {
	db := testopen.SQLite(t)
	ctx := context.Background()
	if _, err := momo.Exec(ctx, db, "CREATE TABLE guid_one (id INTEGER PRIMARY KEY, uid TEXT NOT NULL, owner TEXT)"); err != nil {
		t.Fatal(err)
	}
	a := uuid.MustParse("00000001-0000-0000-0000-000000000000")
	b := uuid.MustParse("00000002-0000-0000-0000-000000000000")
	if _, err := momo.Exec(ctx, db, "INSERT INTO guid_one VALUES (1, ?, ?)", a, &b); err != nil {
		t.Fatal(err)
	}
	if _, err := momo.Exec(ctx, db, "INSERT INTO guid_one VALUES (2, ?, ?)", b, (*uuid.UUID)(nil)); err != nil {
		t.Fatal(err)
	}
	rows, err := momo.Query[momo.Row2[uuid.UUID, *uuid.UUID]](ctx, db, "SELECT uid, owner FROM guid_one ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	got := [][2]any{}
	for _, r := range rows {
		got = append(got, [2]any{r.C1, r.C2})
	}
	want := [][2]any{{a, &b}, {b, (*uuid.UUID)(nil)}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rows = %#v, want %+v", got, want)
	}
}
