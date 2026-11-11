// SPDX-License-Identifier: Apache-2.0

//go:build integration

package jsonop_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/arrayop"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/exprtest"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/jsonop"
	"github.com/azrtydxb/momo/sqlfn"
	"github.com/azrtydxb/momo/types"
)

// Catches: root extraction, key delete, concat, jsonb_set with a computed
// path and the array constructors that render but select or compute the wrong
// rows or values in the database, a path that PostgreSQL cannot type, and an
// operator that renders a lookalike instead of ErrUnsupported on SQLite.
func TestJSONEditOperatorsAgainstDatabase(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		sqlite := d == dialect.SQLite
		ctx := context.Background()
		colType := "jsonb"
		if sqlite {
			colType = "text"
		}
		for _, stmt := range []string{
			"DROP TABLE IF EXISTS jsonedit",
			"CREATE TABLE jsonedit (id bigint PRIMARY KEY, doc " + colType + ", scalar " + colType + ", label " + colType + ")",
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
			insert := "INSERT INTO jsonedit (id, doc, scalar, label) VALUES (?, CAST(? AS jsonb), CAST(? AS jsonb), CAST(? AS jsonb))"
			if sqlite {
				insert = "INSERT INTO jsonedit (id, doc, scalar, label) VALUES (?, ?, ?, ?)"
			}
			if _, err := momo.Exec(ctx, db, insert, r.id, r.doc, r.scalar, r.label); err != nil {
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
		check("root cast to int", ids(sqlfn.Cast[int64](jsonop.Root(scalar), "bigint").Gt(5)), 2, 3)
		check("root of an object is null on a text member", ids(jsonop.Root(doc).IsNull()))

		// DeleteKey keeps every other member and drops the named one.
		check("key present before delete", ids(jsonop.HasKey(doc, "password_hash")), 1)
		check("key gone after delete", ids(jsonop.HasKey(jsonop.DeleteKey(doc, "password_hash"), "password_hash")))
		check("other keys kept", ids(jsonop.HasKey(jsonop.DeleteKey(doc, "password_hash"), "via")), 1, 3)
		check("missing key is a no-op", ids(jsonop.HasKey(jsonop.DeleteKey(doc, "nope"), "via")), 1, 3)
		check("key bound as data", ids(jsonop.HasKey(jsonop.DeleteKey(doc, "via' OR '1'='1"), "via")), 1, 3)
		check("two deletes", ids(jsonop.HasKey(jsonop.DeleteKey(jsonop.DeleteKey(doc, "via"), "a"), "a")))

		if sqlite {
			// json_patch and friends merge recursively and drop nulls, and
			// SQLite cannot compute a json_set path in SQL.
			object := momo.Func[types.JSON[any]]("json_object", "via", "web")
			unsupported("concat on SQLite", jsonop.HasKey(jsonop.Concat(doc, object), "via"))
			unsupported("set on SQLite", jsonop.HasKey(jsonop.Set(doc, momo.ArrayOf[string](momo.Value("k")), momo.Func[types.JSON[int]]("json", 1)), "k"))
			unsupported("array of on SQLite", arrayop.Contains(momo.ArrayOf[int64](id), 1))
			return
		}

		// jsonb_build_object is variadic "any", so its bound arguments carry casts.
		text := func(s string) momo.Expression[string] { return sqlfn.Cast[string](momo.Value(s), "text") }
		object := func(k, v string) momo.Expression[types.JSON[any]] {
			return momo.Func[types.JSON[any]]("jsonb_build_object", text(k), text(v))
		}

		// Concat: the right side wins, the left keeps its other members, and
		// nested objects are replaced, not merged.
		check("concat adds a member", ids(jsonop.Text(jsonop.Concat(doc, object("added", "yes")), "added").Eq("yes")), 1, 2, 3)
		check("concat keeps members", ids(jsonop.Text(jsonop.Concat(doc, object("added", "yes")), "a").Eq("1")), 1)
		check("concat right side wins", ids(jsonop.Text(jsonop.Concat(doc, object("via", "ws")), "via").Eq("ws")), 1, 2, 3)
		nested := momo.Func[types.JSON[any]]("jsonb_build_object", text("skipped"), object("y", "2"))
		check("concat replaces nested objects", ids(jsonop.Path(jsonop.Concat(doc, nested), "skipped", "x").IsNull()), 1, 2, 3)
		check("concat nested value present", ids(jsonop.Path(jsonop.Concat(doc, nested), "skipped", "y").Eq("2")), 1, 2, 3)
		check("remove then add back", ids(jsonop.Text(jsonop.Concat(jsonop.DeleteKey(doc, "password_hash"), object("password_hash", "x")), "password_hash").Eq("x")), 1, 2, 3)

		// Set with a path built in SQL, the way a counter key is computed.
		path := momo.ArrayOf[string](sqlfn.Concat(text("at_"), text("exec")))
		seven := momo.Func[types.JSON[int]]("to_jsonb", sqlfn.Cast[int](momo.Value(7), "int"))
		check("set computed path creates the key", ids(jsonop.Text(jsonop.Set(doc, path, seven), "at_exec").Eq("7")), 1, 2, 3)
		check("set keeps other members", ids(jsonop.Text(jsonop.Set(doc, path, seven), "via").Eq("api")), 1)
		nine := momo.Func[types.JSON[int]]("to_jsonb", sqlfn.Cast[int](momo.Value(9), "int"))
		deep := momo.ArrayOf[string](text("skipped"), text("x"))
		check("set nested path", ids(jsonop.Path(jsonop.Set(doc, deep, nine), "skipped", "x").Eq("9")), 2)

		// ArrayOf and ArraySubquery.
		check("array of column", ids(arrayop.Contains(momo.ArrayOf[int64](id), 1)), 1)
		check("array of two", ids(arrayop.Contains(momo.ArrayOf[int64](id, momo.Value(int64(2))), 2)), 1, 2, 3)
		pick := func(p momo.Predicate) momo.SelectQuery[exprtest.Tally] {
			return momo.Select[exprtest.Tally](db).From(momo.TableRef{Name: "jsonedit"}).Columns(id).Where(p)
		}
		union := momo.Union(false, pick(id.Gt(2)), pick(id.Eq(1)))
		check("array subquery of a union", ids(arrayop.Contains(momo.ArraySubquery[int64](union), 1, 3)), 1, 2, 3)
		check("array subquery misses", ids(arrayop.Contains(momo.ArraySubquery[int64](union), 2)))
		check("array subquery of no rows is an empty array", ids(arrayop.Contains(momo.ArraySubquery[int64](pick(id.Gt(100))))), 1, 2, 3)
	})
}
