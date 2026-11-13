// SPDX-License-Identifier: Apache-2.0

//go:build integration

package types_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/types"
)

// jsonIsNull reports IsNull on v when it has the method; a type without it
// (before the method existed) never reports a JSON null.
func jsonIsNull(v any) bool {
	n, ok := v.(interface{ IsNull() bool })
	return ok && n.IsNull()
}

// storedKind returns "" for SQL NULL, "null" for the JSON literal null and
// "other" for any other value, using the dialect's own JSON type function
// where the column has one.
func storedKind(t *testing.T, db *momo.DB, d dialect.Name, tbl string, id int) string {
	t.Helper()
	var q string
	switch d {
	case dialect.Postgres:
		q = "SELECT jsonb_typeof(v) FROM " + tbl + " WHERE id = ?"
	case dialect.SQLite:
		q = "SELECT json_type(v) FROM " + tbl + " WHERE id = ?"
	default:
		// MySQL: the column is TEXT, so compare the text.
		q = "SELECT CASE WHEN v IS NULL THEN NULL WHEN v = 'null' THEN 'null' ELSE 'other' END FROM " + tbl + " WHERE id = ?"
	}
	got, err := momo.Scalars[*string](context.Background(), db, q, id)
	if err != nil || len(got) != 1 {
		t.Fatalf("stored kind of id %d: %v (%d rows)", id, err, len(got))
	}
	if got[0] == nil {
		return ""
	}
	return *got[0]
}

// jsonNullRoundTrip writes SQL NULL and the JSON literal null into a nullable
// JSON column, reads both back through *types.JSON[*doc], writes what it read
// and checks what is stored.
func jsonNullRoundTrip(t *testing.T, db *momo.DB, d dialect.Name) {
	ctx := context.Background()
	tbl := "json_null_rt"
	if _, err := momo.Exec(ctx, db, "CREATE TABLE "+tbl+" (id int PRIMARY KEY, v "+types.JSON[*doc]{}.SQLType(d)+")"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	// A nil *doc encodes as the JSON literal null.
	if _, err := momo.Exec(ctx, db, "INSERT INTO "+tbl+" (id, v) VALUES (1, ?)", (*types.JSON[*doc])(nil)); err != nil {
		t.Fatalf("insert SQL NULL: %v", err)
	}
	if _, err := momo.Exec(ctx, db, "INSERT INTO "+tbl+" (id, v) VALUES (2, ?)", &types.JSON[*doc]{}); err != nil {
		t.Fatalf("insert JSON null: %v", err)
	}
	if _, err := momo.Exec(ctx, db, "INSERT INTO "+tbl+" (id, v) VALUES (3, ?)", &types.JSON[*doc]{V: &doc{Name: "x"}}); err != nil {
		t.Fatalf("insert object: %v", err)
	}
	for id, want := range map[int]string{1: "", 2: "null", 3: "other"} {
		if got := storedKind(t, db, d, tbl, id); got != want && !(id == 3 && got == "object") {
			t.Fatalf("id %d stored as %q, want %q", id, got, want)
		}
	}

	got, err := momo.Scalars[*types.JSON[*doc]](ctx, db, "SELECT v FROM "+tbl+" ORDER BY id")
	if err != nil || len(got) != 3 {
		t.Fatalf("select: %v (%d rows)", err, len(got))
	}
	if got[0] != nil {
		t.Fatalf("SQL NULL read as %+v, want a nil pointer", got[0])
	}
	if got[1] == nil || !jsonIsNull(got[1]) {
		t.Fatalf("JSON null read as %+v, want a non-nil value with IsNull true", got[1])
	}
	if got[2] == nil || jsonIsNull(got[2]) || got[2].V == nil || got[2].V.Name != "x" {
		t.Fatalf("object read as %+v", got[2])
	}

	// Write what was read, under new ids.
	for i, g := range got {
		if _, err := momo.Exec(ctx, db, "INSERT INTO "+tbl+" (id, v) VALUES (?, ?)", 10+i, g); err != nil {
			t.Fatalf("write back row %d: %v", i, err)
		}
	}
	for id, want := range map[int]string{10: "", 11: "null"} {
		if got := storedKind(t, db, d, tbl, id); got != want {
			t.Fatalf("written back id %d stored as %q, want %q", id, got, want)
		}
	}
}

// Catches: a nullable JSON column that reads the JSON literal null as a nil
// pointer, so SQL NULL and JSON null cannot be told apart and writing the
// read value back turns JSON null into SQL NULL (issue #118).
func TestJSONNullVersusSQLNullLive(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		jsonNullRoundTrip(t, db, d)
	})
}

// Catches: the same loss on MySQL 8.4, 9.7 and MariaDB, where the column is
// TEXT.
func TestJSONNullVersusSQLNullLiveMySQL(t *testing.T) {
	for _, s := range mysqlServers {
		t.Run(s.name, func(t *testing.T) {
			db := s.open(t)
			jsonNullRoundTrip(t, db, db.Dialect().Name)
		})
	}
}

// jsonStructRoundTrip uses a struct T, whose zero value encodes as an object,
// so a rewrite that lost the null marker would store {"name":"","tags":null}
// and not null. It also reads through momo.Query into a plain struct, with one
// pointer and one non-pointer JSON field.
func jsonStructRoundTrip(t *testing.T, db *momo.DB, d dialect.Name) {
	ctx := context.Background()
	tbl := "json_null_struct"
	typ := types.JSON[doc]{}.SQLType(d)
	if _, err := momo.Exec(ctx, db, "CREATE TABLE "+tbl+" (id int PRIMARY KEY, ptr "+typ+", val "+typ+" NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	if _, err := momo.Exec(ctx, db, "INSERT INTO "+tbl+" (id, ptr, val) VALUES (1, ?, ?)", (*types.JSON[doc])(nil), types.JSONNull[doc]()); err != nil {
		t.Fatalf("insert row 1: %v", err)
	}
	if _, err := momo.Exec(ctx, db, "INSERT INTO "+tbl+" (id, ptr, val) VALUES (2, ?, ?)", types.JSONNull[doc](), types.JSON[doc]{V: doc{Name: "x"}}); err != nil {
		t.Fatalf("insert row 2: %v", err)
	}

	type row struct {
		ID  int
		Ptr *types.JSON[doc]
		Val types.JSON[doc]
	}
	rows, err := momo.Query[row](ctx, db, "SELECT id, ptr, val FROM "+tbl+" ORDER BY id")
	if err != nil || len(rows) != 2 {
		t.Fatalf("Query: %v (%d rows)", err, len(rows))
	}
	if rows[0].Ptr != nil || !jsonIsNull(rows[0].Val) {
		t.Fatalf("row 1: ptr %+v, val %+v; want a nil pointer and a JSON null", rows[0].Ptr, rows[0].Val)
	}
	if rows[1].Ptr == nil || !jsonIsNull(rows[1].Ptr) || jsonIsNull(rows[1].Val) || rows[1].Val.V.Name != "x" {
		t.Fatalf("row 2: ptr %+v, val %+v", rows[1].Ptr, rows[1].Val)
	}

	// Store the values just read under a new id; a lost marker would store an object.
	if _, err := momo.Exec(ctx, db, "INSERT INTO "+tbl+" (id, ptr, val) VALUES (3, ?, ?)", rows[1].Ptr, rows[0].Val); err != nil {
		t.Fatalf("second insert: %v", err)
	}
	for _, col := range []string{"ptr", "val"} {
		q := "SELECT CASE WHEN " + col + " IS NULL THEN NULL WHEN " + col + " = 'null' THEN 'null' ELSE 'other' END FROM " + tbl + " WHERE id = 3"
		if d == dialect.Postgres {
			q = "SELECT jsonb_typeof(" + col + ") FROM " + tbl + " WHERE id = 3"
		}
		got, err := momo.Scalars[string](ctx, db, q)
		if err != nil || len(got) != 1 || got[0] != "null" {
			t.Fatalf("stored %s: %v, %v; want the JSON null", col, got, err)
		}
	}
}

// Catches: a struct T whose JSON null is stored as the encoding of its zero
// value (an object) when the null marker is dropped, and Query[T] into a plain
// struct reading the JSON null of a pointer or non-pointer field wrongly.
func TestJSONNullStructLive(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		jsonStructRoundTrip(t, db, d)
	})
}

// Catches: the same on MySQL 8.4, 9.7 and MariaDB.
func TestJSONNullStructLiveMySQL(t *testing.T) {
	for _, s := range mysqlServers {
		t.Run(s.name, func(t *testing.T) {
			db := s.open(t)
			jsonStructRoundTrip(t, db, db.Dialect().Name)
		})
	}
}
