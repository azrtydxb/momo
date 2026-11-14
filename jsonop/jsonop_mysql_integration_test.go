// SPDX-License-Identifier: Apache-2.0

//go:build integration

package jsonop_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/exprtest"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/jsonop"
	"github.com/azrtydxb/momo/sqlfn"
	"github.com/azrtydxb/momo/types"
)

// Catches: JSON operators that render for MySQL or MariaDB but match the
// wrong rows on the server, in particular JSON_CONTAINS with a CAST(? AS JSON)
// candidate, which MariaDB does not parse, and a confusion of SQL NULL, JSON
// null and a missing key (JSON_UNQUOTE turns a JSON null into the text null).
func TestJSONOperatorsAgainstMySQLFamily(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		ctx := context.Background()
		for _, stmt := range []string{
			"DROP TABLE IF EXISTS jsonev",
			"CREATE TABLE jsonev (id bigint PRIMARY KEY, doc json NULL)",
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
			if _, err := momo.Exec(ctx, db, "INSERT INTO jsonev (id, doc) VALUES (?, ?)", r.id, r.doc); err != nil {
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
		check("contains object", ids(jsonop.Contains(doc, map[string]any{"via": "web"})), 2)
		check("contains array member", ids(jsonop.Contains(doc, map[string]any{"tags": []string{"a"}})), 1)
		check("contains array", ids(jsonop.Contains(doc, []string{"a"})), 6)
		check("contains scalar", ids(jsonop.Contains(doc, "a")), 6, 7)

		// A JSON boolean and a number read as their JSON text.
		check("boolean true as text", ids(jsonop.Text(doc, "flag").Eq("true")), 8)
		check("boolean false as text", ids(jsonop.Text(doc, "off").Eq("false")), 8)
		check("number as text", ids(jsonop.Text(doc, "num").Eq("1.5")), 8)
		check("has key", ids(jsonop.HasKey(doc, "via")), 1, 2)
		check("has key with JSON null value", ids(jsonop.HasKey(doc, "gone")), 2)
		check("key bound as data", ids(jsonop.HasKey(doc, "via' OR '1'='1")))

		// A numeric key is never an array index outside Path.
		check("text key 1 on an array", ids(jsonop.Text(doc, "1").Eq("b")))
		check("has key 1 on an array", ids(jsonop.HasKey(doc, "1")))
		check("path element 1 on an array", ids(jsonop.Path(doc, "1").Eq("b")), 6)
		check("has key on array and scalar", ids(jsonop.HasKey(doc, "a")))

		// Grouping by an extraction works for a plain key, whose path is inlined.
		e := jsonop.Text(doc, "via")
		g, err := momo.Select[strCount](db).From(momo.TableRef{Name: "jsonev"}).
			Columns(e, momo.CountAll()).Where(jsonop.HasKey(doc, "via")).GroupBy(e).OrderBy(momo.Asc(e)).All(ctx)
		if err != nil || len(g) != 2 || g[0].S != "api" || g[0].N != 1 || g[1].S != "web" || g[1].N != 1 {
			t.Errorf("group by plain key = %+v, %v", g, err)
		}

		// Keys that look like indexes, paths or operators are plain object keys.
		check("numeric object key", ids(jsonop.Text(doc, "0").Eq("zero")), 5)
		check("backslash key", ids(jsonop.Text(doc, `a\b`).Eq("bs")), 5)
		check("has key backslash", ids(jsonop.HasKey(doc, `a\b`)), 5)
		check("question mark key", ids(jsonop.Text(doc, "q?").Eq("x")), 5)
		check("dotted key", ids(jsonop.Text(doc, "a.b").Eq("dot")), 5)
		check("has key question mark", ids(jsonop.HasKey(doc, "q?")), 5)
		check("path with dotted element", ids(jsonop.Path(doc, "a.b").Eq("dot")), 5)

		// SQL NULL and a missing key extract as NULL; a JSON null member is the
		// text null, because JSON_UNQUOTE turns it into text.
		check("extracted text is NULL", ids(jsonop.Text(doc, "via").IsNull()), 3, 4, 5, 6, 7, 8)
		check("missing key is NULL, JSON null member is not", ids(jsonop.Text(doc, "gone").IsNull()), 1, 3, 4, 5, 6, 7, 8)
		check("JSON null member reads as the text null", ids(jsonop.Text(doc, "gone").Eq("null")), 2)
		check("SQL NULL column", ids(doc.IsNull()), 3)

		check("order desc", ids(jsonop.HasKey(doc, "via"), jsonop.Text(doc, "via").Desc()), 2, 1)
		check("order asc", ids(jsonop.HasKey(doc, "via"), jsonop.Text(doc, "via").Asc()), 1, 2)
	})
}

// Catches: Root, DeleteKey, Concat and Set rendering for MySQL or MariaDB SQL
// that selects or computes the wrong rows, or that renders a lookalike for
// Concat and Set (which MySQL cannot express with the PostgreSQL rules)
// instead of ErrUnsupported.
func TestJSONEditOperatorsAgainstMySQLFamily(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		ctx := context.Background()
		for _, stmt := range []string{
			"DROP TABLE IF EXISTS jsonedit",
			"CREATE TABLE jsonedit (id bigint PRIMARY KEY, doc json, scalar json, label json)",
		} {
			if _, err := momo.Exec(ctx, db, stmt); err != nil {
				t.Fatal(err)
			}
		}
		rows := []struct {
			id                 int64
			doc, scalar, label string
		}{
			{1, `{"a":1,"password_hash":"h","via":"api"}`, `5`, `"hello"`},
			{2, `{"skipped":{"x":1}}`, `9`, `"world"`},
			{3, `{"via":"web"}`, `7`, `"x y"`},
		}
		for _, r := range rows {
			if _, err := momo.Exec(ctx, db, "INSERT INTO jsonedit (id, doc, scalar, label) VALUES (?, ?, ?, ?)", r.id, r.doc, r.scalar, r.label); err != nil {
				t.Fatal(err)
			}
		}

		doc := momo.NewColumn[types.JSON[any]]("jsonedit", "doc", 0)
		scalar := momo.NewColumn[types.JSON[any]]("jsonedit", "scalar", 0)
		label := momo.NewColumn[types.JSON[any]]("jsonedit", "label", 0)
		id := momo.ColumnOf[int64]("jsonedit", "id")
		ids := func(p momo.Predicate) []int64 {
			t.Helper()
			got, err := momo.Select[exprtest.Pair](db).From(momo.TableRef{Name: "jsonedit"}).
				Columns(id, id).Where(p).OrderBy(id.Asc()).All(ctx)
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
		unsupported := func(name string, p momo.Predicate) {
			t.Helper()
			_, err := momo.Select[exprtest.Pair](db).From(momo.TableRef{Name: "jsonedit"}).
				Columns(id, id).Where(p).All(ctx)
			if !errors.Is(err, momo.ErrUnsupported) {
				t.Errorf("%s = %v, want ErrUnsupported", name, err)
			}
		}

		// Root: a scalar stored at the root of the value.
		check("root number", ids(jsonop.Root(scalar).Eq("5")), 1)
		check("root number as the second row", ids(jsonop.Root(scalar).Eq("9")), 2)
		check("root string is unquoted", ids(jsonop.Root(label).Eq("hello")), 1)
		check("root string with a space", ids(jsonop.Root(label).Eq("x y")), 3)
		check("root cast to int", ids(sqlfn.Cast[int64](jsonop.Root(scalar), "signed").Gt(5)), 2, 3)
		check("root of an object is its JSON text, not NULL", ids(jsonop.Root(doc).IsNull()))

		// DeleteKey keeps every other member and drops the named one.
		check("key present before delete", ids(jsonop.HasKey(doc, "password_hash")), 1)
		check("key gone after delete", ids(jsonop.HasKey(jsonop.DeleteKey(doc, "password_hash"), "password_hash")))
		check("other keys kept", ids(jsonop.HasKey(jsonop.DeleteKey(doc, "password_hash"), "via")), 1, 3)
		check("missing key is a no-op", ids(jsonop.HasKey(jsonop.DeleteKey(doc, "nope"), "via")), 1, 3)
		check("key bound as data", ids(jsonop.HasKey(jsonop.DeleteKey(doc, "via' OR '1'='1"), "via")), 1, 3)
		check("two deletes", ids(jsonop.HasKey(jsonop.DeleteKey(jsonop.DeleteKey(doc, "via"), "a"), "a")))

		// Concat and Set have no MySQL operator with the PostgreSQL rules.
		object := momo.Func[types.JSON[any]]("JSON_OBJECT", "via", "web")
		unsupported("concat", jsonop.HasKey(jsonop.Concat(doc, object), "via"))
		unsupported("set", jsonop.HasKey(jsonop.Set(doc, momo.ArrayOf[string](momo.Value("k")), momo.Func[types.JSON[int]]("JSON_QUOTE", "1")), "k"))
		_, err := momo.Select[exprtest.Tally](db).From(momo.TableRef{Name: "jsonedit"}).
			Columns(momo.ArrayOf[int64](id)).All(ctx)
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("ArrayOf = %v, want ErrUnsupported", err)
		}
	})
}
