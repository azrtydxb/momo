// SPDX-License-Identifier: Apache-2.0

//go:build integration

package jsonop_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/exprtest"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/jsonop"
	"github.com/azrtydxb/momo/types"
)

// strCount is a projection of a string key and a count.
type strCount struct {
	S string
	N int64
}

var strCountMeta = momo.Meta[strCount]{
	Columns:  []momo.ColumnMeta{{Name: "s"}, {Name: "n"}},
	Ptr:      func(m *strCount, i int) any { return [...]any{&m.S, &m.N}[i] },
	Value:    func(m *strCount, i int) any { return [...]any{m.S, m.N}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*strCount) MomoMeta() *momo.Meta[strCount] { return &strCountMeta }

// Catches: JSON operators that render but do not match the rows a jsonb
// column holds (wrong operator, bound value mis-typed), and a confusion of SQL
// NULL, JSON null and a missing key.
func TestJSONOperatorsAgainstDatabase(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		sqlite := d == dialect.SQLite
		ctx := context.Background()
		for _, stmt := range []string{
			"DROP TABLE IF EXISTS jsonev",
			"CREATE TABLE jsonev (id bigint PRIMARY KEY, doc " + map[bool]string{true: "text", false: "jsonb"}[sqlite] + ")",
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
			{5, `{"0":"zero","a\\b":"bs","q?":"x","a.b":"dot"}`},
			{6, `["a","b"]`},
			{7, `"a"`},
			{8, `{"flag":true,"off":false,"num":1.5}`},
		}
		for _, r := range rows {
			insert := "INSERT INTO jsonev (id, doc) VALUES (?, CAST(? AS jsonb))"
			if sqlite {
				// CAST(x AS jsonb) would be a numeric cast on SQLite; the column is text.
				insert = "INSERT INTO jsonev (id, doc) VALUES (?, ?)"
			}
			if _, err := momo.Exec(ctx, db, insert, r.id, r.doc); err != nil {
				t.Fatal(err)
			}
		}

		doc := momo.NewNullColumn[types.JSON[any]]("jsonev", "doc", 0)
		id := momo.ColumnOf[int64]("jsonev", "id")
		ids := func(p momo.Predicate, order ...momo.Orderable) []int64 {
			t.Helper()
			q := momo.Select[exprtest.Pair](db).From(momo.TableRef{Name: "jsonev"}).Columns(id, id).Where(p)
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

		check("text eq", ids(jsonop.Text(doc, "via").Eq("api")), 1)
		check("text in", ids(jsonop.Text(doc, "via").In("api", "web")), 1, 2)
		check("path", ids(jsonop.Path(doc, "n", "x", "1").Eq("q")), 1)
		if sqlite {
			// SQLite has no containment operator, and json_each cannot express
			// the recursive @> rule, so Contains stays ErrUnsupported there.
			_, err := momo.Select[exprtest.Pair](db).From(momo.TableRef{Name: "jsonev"}).Columns(id, id).
				Where(jsonop.Contains(doc, map[string]any{"via": "web"})).All(ctx)
			if !errors.Is(err, momo.ErrUnsupported) {
				t.Errorf("Contains on SQLite = %v, want ErrUnsupported", err)
			}
		} else {
			check("contains object", ids(jsonop.Contains(doc, map[string]any{"via": "web"})), 2)
			check("contains array member", ids(jsonop.Contains(doc, map[string]any{"tags": []string{"a"}})), 1)
		}
		// A JSON boolean reads as true or false on PostgreSQL and as 1 or 0 on
		// SQLite, whose json_extract returns an integer; numbers read the same.
		yes, no := "true", "false"
		if sqlite {
			yes, no = "1", "0"
		}
		check("boolean true as text", ids(jsonop.Text(doc, "flag").Eq(yes)), 8)
		check("boolean false as text", ids(jsonop.Text(doc, "off").Eq(no)), 8)
		check("number as text", ids(jsonop.Text(doc, "num").Eq("1.5")), 8)
		check("has key", ids(jsonop.HasKey(doc, "via")), 1, 2)
		check("has key with JSON null value", ids(jsonop.HasKey(doc, "gone")), 2)
		check("key bound as data", ids(jsonop.HasKey(doc, "via' OR '1'='1")))

		// A numeric key is never an array index outside Path.
		check("text key 1 on an array", ids(jsonop.Text(doc, "1").Eq("b")))
		check("has key 1 on an array", ids(jsonop.HasKey(doc, "1")))
		check("path element 1 on an array", ids(jsonop.Path(doc, "1").Eq("b")), 6)

		// Grouping by an extraction: a plain key works, any other key fails
		// on PostgreSQL because each clause binds its own parameter.
		group := func(key string) ([]strCount, error) {
			e := jsonop.Text(doc, key)
			return momo.Select[strCount](db).From(momo.TableRef{Name: "jsonev"}).
				Columns(e, momo.CountAll()).Where(jsonop.HasKey(doc, "via")).GroupBy(e).OrderBy(momo.Asc(e)).All(ctx)
		}
		if g, err := group("via"); err != nil || len(g) != 2 || g[0].S != "api" || g[0].N != 1 || g[1].S != "web" || g[1].N != 1 {
			t.Errorf("group by plain key = %+v, %v", g, err)
		}
		if _, err := group("a.b"); err == nil && !sqlite {
			t.Error("group by a bound key succeeded; the documented caveat no longer holds")
		}

		// Keys that look like indexes, paths or operators are plain object keys.
		check("numeric object key", ids(jsonop.Text(doc, "0").Eq("zero")), 5)
		if sqlite {
			// SQLite JSON paths cannot carry a backslash, so the key is refused.
			_, err := momo.Select[exprtest.Pair](db).From(momo.TableRef{Name: "jsonev"}).Columns(id, id).
				Where(jsonop.Text(doc, `a\b`).Eq("bs")).All(ctx)
			if !errors.Is(err, momo.ErrValidation) {
				t.Errorf("backslash key on SQLite = %v, want ErrValidation", err)
			}
		} else {
			check("backslash key", ids(jsonop.Text(doc, `a\b`).Eq("bs")), 5)
			check("has key backslash", ids(jsonop.HasKey(doc, `a\b`)), 5)
		}
		check("question mark key", ids(jsonop.Text(doc, "q?").Eq("x")), 5)
		check("dotted key", ids(jsonop.Text(doc, "a.b").Eq("dot")), 5)
		check("has key question mark", ids(jsonop.HasKey(doc, "q?")), 5)
		check("path with dotted element", ids(jsonop.Path(doc, "a.b").Eq("dot")), 5)
		check("path array index", ids(jsonop.Path(doc, "1").Eq("b")), 6)

		// Containment with array and scalar top-level values.
		if sqlite {
			// json_type of a member path is NULL on arrays and scalars.
			check("has key on array and scalar", ids(jsonop.HasKey(doc, "a")))
		} else {
			check("contains array", ids(jsonop.Contains(doc, []string{"a"})), 6)
			check("contains scalar", ids(jsonop.Contains(doc, "a")), 6, 7)
			check("has key on array and scalar", ids(jsonop.HasKey(doc, "a")), 6, 7)
		}

		// SQL NULL, JSON null and a missing key all extract as NULL.
		check("extracted text is NULL", ids(jsonop.Text(doc, "via").IsNull()), 3, 4, 5, 6, 7, 8)
		check("missing key and JSON null member are both NULL", ids(jsonop.Text(doc, "gone").IsNull()), 1, 2, 3, 4, 5, 6, 7, 8)
		check("SQL NULL column", ids(doc.IsNull()), 3)
		check("JSON null column", ids(doc.Eq(types.JSON[any]{})), 4)

		check("order desc", ids(jsonop.HasKey(doc, "via"), jsonop.Text(doc, "via").Desc()), 2, 1)
		check("order asc", ids(jsonop.HasKey(doc, "via"), jsonop.Text(doc, "via").Asc()), 1, 2)
	})
}
