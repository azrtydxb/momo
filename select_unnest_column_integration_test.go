// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// Catches: UnnestColumn not expanding a text[] column of an earlier source,
// or the lateral reference to it not resolving (PostgreSQL only).
func TestUnnestColumnExpandsArrayColumn(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		ctx := context.Background()
		flags := momo.ColumnOf[[]string]("o", "flags")
		src := momo.UnnestColumn(flags).As("fl", "flag")
		id := momo.ColumnOf[int64]("o", "id")
		q := momo.Select[anKT](db).From(momo.TableRef{Name: "an_owners", Alias: "o"}).
			Columns(id, src.Column()).
			CrossJoin(src).OrderBy(id.Asc(), src.Column().Asc())
		if d != dialect.Postgres {
			if _, err := q.SQL(); !errors.Is(err, momo.ErrUnsupported) {
				t.Fatalf("%s: err = %v, want ErrUnsupported", d, err)
			}
			return
		}
		for _, stmt := range []string{
			"DROP TABLE IF EXISTS an_owners",
			"CREATE TABLE an_owners (id bigint PRIMARY KEY, flags text[])",
			"INSERT INTO an_owners VALUES (1, ARRAY['x','y','x']), (2, NULL), (3, ARRAY['z'])",
		} {
			if _, err := momo.Exec(ctx, db, stmt); err != nil {
				t.Fatal(err)
			}
		}
		t.Cleanup(func() { _, _ = momo.Exec(context.Background(), db, "DROP TABLE IF EXISTS an_owners") })
		rows, err := q.All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		want := []anKT{{1, "x"}, {1, "x"}, {1, "y"}, {3, "z"}}
		if len(rows) != len(want) {
			t.Fatalf("rows = %+v, want %+v", rows, want)
		}
		for i := range want {
			if rows[i] != want[i] {
				t.Fatalf("rows = %+v, want %+v", rows, want)
			}
		}
	})
}
