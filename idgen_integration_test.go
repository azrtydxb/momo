// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// TestIDGenLive inserts models with zero ids into a real table with no
// database default for the primary key, through every model insert path, and
// reads the rows back by the ids that were written into the models.
func TestIDGenLive(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		ctx := context.Background()
		for _, stmt := range []string{
			"DROP TABLE IF EXISTS id_rows",
			"CREATE TABLE id_rows (id text primary key, name text not null)",
		} {
			if _, err := db.Driver().Exec(ctx, stmt, nil); err != nil {
				t.Fatalf("%s: %v", stmt, err)
			}
		}
		count := func() int {
			rows, err := db.Driver().Query(ctx, "SELECT count(*) FROM id_rows", nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = rows.Close() }()
			var n int
			if !rows.Next() {
				t.Fatal("no count row")
			}
			if err := rows.Scan(&n); err != nil {
				t.Fatal(err)
			}
			return n
		}
		stored := func(id string) bool {
			rows, err := db.Driver().Query(ctx, "SELECT name FROM id_rows WHERE id = "+placeholder(db), []any{id})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = rows.Close() }()
			return rows.Next()
		}

		// Model
		one := idRow{Name: "one"}
		if _, err := momo.Insert[idRow](db).Model(&one).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		if !reCUID.MatchString(one.ID) || !stored(one.ID) {
			t.Fatalf("Model: id %q not stored", one.ID)
		}

		// Models
		many := []idRow{{Name: "a"}, {ID: "kept", Name: "b"}, {Name: "c"}}
		if _, err := momo.Insert[idRow](db).Models(many).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		if many[1].ID != "kept" || !stored("kept") {
			t.Fatalf("an assigned id must be kept: %q", many[1].ID)
		}
		for _, i := range []int{0, 2} {
			if !reCUID.MatchString(many[i].ID) || !stored(many[i].ID) {
				t.Fatalf("Models[%d]: id %q not stored", i, many[i].ID)
			}
		}

		// Chunked bulk insert: every chunk binds the id written to its model.
		chunked := make([]idRow, 41)
		for i := range chunked {
			chunked[i].Name = "chunk"
		}
		limited := momo.New(db.Driver(), momo.WithMaxParams(20))
		if _, err := momo.Insert[idRow](limited).Models(chunked).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		for i := range chunked {
			if !stored(chunked[i].ID) {
				t.Fatalf("chunked[%d]: id %q not stored", i, chunked[i].ID)
			}
		}

		// Upsert: a zero id inserts a new row under a generated id.
		up := idRow{Name: "up"}
		if _, err := momo.Upsert[idRow](db).Model(&up).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		if !reCUID.MatchString(up.ID) || !stored(up.ID) {
			t.Fatalf("Upsert: id %q not stored", up.ID)
		}
		// Upserting the same model again hits the row by its id.
		before := count()
		up.Name = "up again"
		if _, err := momo.Upsert[idRow](db).Model(&up).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		if count() != before {
			t.Fatal("upsert with an assigned id must update, not insert")
		}

		// Returning rows carry the generated id.
		ret := idRow{Name: "ret"}
		rows, err := momo.Insert[idRow](db).Model(&ret).ReturningAll().Rows(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var got []idRow
		for rows.Next() {
			var r idRow
			if err := rows.ScanModel(&r); err != nil {
				t.Fatal(err)
			}
			got = append(got, r)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].ID != ret.ID || !reCUID.MatchString(ret.ID) {
			t.Fatalf("Returning: %+v, model id %q", got, ret.ID)
		}

		want := 1 + 3 + 41 + 1 + 1
		if n := count(); n != want {
			t.Fatalf("rows = %d, want %d", n, want)
		}
	})
}

// placeholder is the bind marker of db's dialect for the first argument.
func placeholder(db *momo.DB) string {
	if db.Driver().Dialect().Name == dialect.Postgres {
		return "$1"
	}
	return "?"
}
