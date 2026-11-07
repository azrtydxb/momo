// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/types"
)

type jsonPayload = map[string]any

var (
	jsonAfter  = momo.NewColumn[types.JSON[jsonPayload]]("events", "after", momo.FlagSensitive)
	jsonBefore = momo.NewNullColumn[types.JSON[jsonPayload]]("events", "before", 0)
)

func jsonWhere(d dialect.Name, p momo.Predicate) (momo.Compiled, error) {
	return momo.Select[unitTally](typedDB(d)).From(momo.TableRef{Name: "events"}).
		Columns(momo.CountAll()).Where(p).SQL()
}

// Catches: a key, path or containment value interpolated instead of bound, a
// wrong operator per dialect, and a missing cast on the containment value.
func TestJSONOperatorsGolden(t *testing.T) {
	cases := []struct {
		name string
		d    dialect.Name
		p    momo.Predicate
		want string
		args []any
	}{
		{"pg text eq", dialect.Postgres, momo.JSONText(jsonAfter, "via").Eq("api"),
			`("events"."after" ->> $1) = $2`, []any{"via", "api"}},
		{"pg text on nullable", dialect.Postgres, momo.JSONText(jsonBefore, "a'b").Ne("x"),
			`("events"."before" ->> $1) <> $2`, []any{"a'b", "x"}},
		{"pg text in", dialect.Postgres, momo.JSONText(jsonAfter, "via").In("a", "b"),
			`("events"."after" ->> $1) IN ($2, $3)`, []any{"via", "a", "b"}},
		{"pg text is null", dialect.Postgres, momo.JSONText(jsonAfter, "via").IsNull(),
			`("events"."after" ->> $1) IS NULL`, []any{"via"}},
		{"pg path", dialect.Postgres, momo.JSONPath(jsonAfter, "a", "0", "b").Eq("x"),
			`("events"."after" #>> $1) = $2`, []any{[]string{"a", "0", "b"}, "x"}},
		{"pg contains", dialect.Postgres, momo.JSONContains(jsonAfter, map[string]any{"via": "api"}),
			`("events"."after" @> CAST($1 AS jsonb))`, []any{`{"via":"api"}`}},
		{"pg has key", dialect.Postgres, momo.JSONHasKey(jsonAfter, "via"),
			`("events"."after" ? $1)`, []any{"via"}},
		{"sqlite text", dialect.SQLite, momo.JSONText(jsonAfter, "via").Eq("api"),
			`CAST(json_extract("events"."after", ?) AS TEXT) = ?`, []any{`$."via"`, "api"}},
		{"sqlite path", dialect.SQLite, momo.JSONPath(jsonAfter, "a", "0", "b").Eq("x"),
			`CAST(json_extract("events"."after", ?) AS TEXT) = ?`, []any{`$."a"[0]."b"`, "x"}},
		{"sqlite has key", dialect.SQLite, momo.JSONHasKey(jsonAfter, "via"),
			`(json_type("events"."after", ?) IS NOT NULL)`, []any{`$."via"`}},
		{"mysql text", dialect.MySQL, momo.JSONText(jsonAfter, "via").Eq("api"),
			"JSON_UNQUOTE(JSON_EXTRACT(`events`.`after`, ?)) = ?", []any{`$."via"`, "api"}},
		{"mysql contains", dialect.MySQL, momo.JSONContains(jsonAfter, []string{"a"}),
			"JSON_CONTAINS(`events`.`after`, CAST(? AS JSON))", []any{`["a"]`}},
		{"mysql has key", dialect.MySQL, momo.JSONHasKey(jsonAfter, "via"),
			"JSON_CONTAINS_PATH(`events`.`after`, 'one', ?)", []any{`$."via"`}},
	}
	for _, c := range cases {
		got, err := jsonWhere(c.d, c.p)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		want := "SELECT COUNT(*) FROM \"events\" WHERE " + c.want
		if c.d == dialect.MySQL {
			want = "SELECT COUNT(*) FROM `events` WHERE " + c.want
		}
		if got.SQL != want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got.SQL, want)
		}
		if !reflect.DeepEqual(got.Args, c.args) {
			t.Errorf("%s: args = %#v, want %#v", c.name, got.Args, c.args)
		}
	}
}

// Catches: the extraction unusable in ORDER BY or the select list.
func TestJSONTextOrderAndColumns(t *testing.T) {
	e := momo.JSONText(jsonAfter, "via")
	got, err := momo.Select[unitTally](typedDB(dialect.Postgres)).From(momo.TableRef{Name: "events"}).
		Columns(e).OrderBy(e.Desc()).SQL()
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT ("events"."after" ->> $1) FROM "events" ORDER BY ("events"."after" ->> $2) DESC`
	if got.SQL != want {
		t.Errorf("got  %s\nwant %s", got.SQL, want)
	}
}

// Catches: sensitivity lost through extraction, and the extraction not
// marked nullable for keyset ordering.
func TestJSONFlags(t *testing.T) {
	f := momo.JSONText(jsonAfter, "via").Flags()
	if f&momo.FlagSensitive == 0 || f&momo.FlagNullable == 0 {
		t.Errorf("JSONText flags = %b, want sensitive and nullable", f)
	}
	if momo.JSONText(jsonBefore, "via").Flags()&momo.FlagSensitive != 0 {
		t.Error("extraction from a non-sensitive column is marked sensitive")
	}
	if momo.JSONPath(jsonAfter, "a").Flags()&momo.FlagSensitive == 0 {
		t.Error("JSONPath lost the sensitive flag")
	}
}

// Catches: empty keys and unrepresentable quotes reaching the database, and
// containment silently rendering on SQLite.
func TestJSONOperatorErrors(t *testing.T) {
	cases := []struct {
		name string
		d    dialect.Name
		p    momo.Predicate
		kind error
	}{
		{"empty key", dialect.Postgres, momo.JSONText(jsonAfter, "").Eq("x"), momo.ErrValidation},
		{"empty path", dialect.Postgres, momo.JSONPath(jsonAfter).Eq("x"), momo.ErrValidation},
		{"quote on sqlite", dialect.SQLite, momo.JSONText(jsonAfter, `a"b`).Eq("x"), momo.ErrValidation},
		{"quote on postgres is fine", dialect.Postgres, momo.JSONText(jsonAfter, `a"b`).Eq("x"), nil},
		{"has key empty", dialect.Postgres, momo.JSONHasKey(jsonAfter, ""), momo.ErrValidation},
		{"contains on sqlite", dialect.SQLite, momo.JSONContains(jsonAfter, jsonPayload{"a": 1}), momo.ErrUnsupported},
		{"unencodable value", dialect.Postgres, momo.JSONContains(jsonAfter, make(chan int)), momo.ErrValidation},
	}
	for _, c := range cases {
		_, err := jsonWhere(c.d, c.p)
		if c.kind == nil {
			if err != nil {
				t.Errorf("%s: %v", c.name, err)
			}
			continue
		}
		if !errors.Is(err, c.kind) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.kind)
		}
	}
}
