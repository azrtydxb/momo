// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"slices"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/types"
)

// Catches: JSON operators that render but do not match the rows a jsonb
// column holds (wrong operator, bound value mis-typed), and a confusion of SQL
// NULL, JSON null and a missing key.
func TestJSONOperatorsAgainstDatabase(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		ctx := context.Background()
		for _, stmt := range []string{
			"DROP TABLE IF EXISTS jsonev",
			"CREATE TABLE jsonev (id bigint PRIMARY KEY, doc jsonb)",
		} {
			if _, err := momo.Exec(ctx, db, stmt); err != nil {
				t.Fatal(err)
			}
		}
		rows := []struct {
			id  int64
			doc any
		}{
			{1, `{"via":"api","n":{"x":["p","q"]},"tags":["a","b"]}`},
			{2, `{"via":"web","gone":null}`},
			{3, nil},
			{4, `null`},
		}
		for _, r := range rows {
			if _, err := momo.Exec(ctx, db, "INSERT INTO jsonev (id, doc) VALUES (?, CAST(? AS jsonb))", r.id, r.doc); err != nil {
				t.Fatal(err)
			}
		}

		doc := momo.NewNullColumn[types.JSON[any]]("jsonev", "doc", 0)
		id := momo.ColumnOf[int64]("jsonev", "id")
		ids := func(p momo.Predicate, order ...momo.Orderable) []int64 {
			t.Helper()
			q := momo.Select[kvRow](db).From(momo.TableRef{Name: "jsonev"}).Columns(id, id).Where(p)
			if len(order) == 0 {
				order = []momo.Orderable{id.Asc()}
			}
			got, err := q.OrderBy(order...).All(ctx)
			if err != nil {
				t.Fatalf("query: %v", err)
			}
			out := make([]int64, len(got))
			for i, r := range got {
				out[i] = r.K
			}
			return out
		}
		check := func(name string, got []int64, want ...int64) {
			t.Helper()
			if !slices.Equal(got, want) {
				t.Errorf("%s = %v, want %v", name, got, want)
			}
		}

		check("text eq", ids(momo.JSONText(doc, "via").Eq("api")), 1)
		check("text in", ids(momo.JSONText(doc, "via").In("api", "web")), 1, 2)
		check("path", ids(momo.JSONPath(doc, "n", "x", "1").Eq("q")), 1)
		check("contains object", ids(momo.JSONContains(doc, map[string]any{"via": "web"})), 2)
		check("contains array member", ids(momo.JSONContains(doc, map[string]any{"tags": []string{"a"}})), 1)
		check("has key", ids(momo.JSONHasKey(doc, "via")), 1, 2)
		check("has key with JSON null value", ids(momo.JSONHasKey(doc, "gone")), 2)
		check("key bound as data", ids(momo.JSONHasKey(doc, "via' OR '1'='1")))

		// SQL NULL, JSON null and a missing key all extract as NULL.
		check("extracted text is NULL", ids(momo.JSONText(doc, "via").IsNull()), 2, 3, 4)
		check("SQL NULL column", ids(doc.IsNull()), 3)
		check("JSON null column", ids(doc.Eq(types.JSON[any]{})), 4)

		check("order desc", ids(momo.JSONHasKey(doc, "via"), momo.JSONText(doc, "via").Desc()), 2, 1)
		check("order asc", ids(momo.JSONHasKey(doc, "via"), momo.JSONText(doc, "via").Asc()), 1, 2)
	})
}
