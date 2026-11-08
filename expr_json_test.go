// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/types"
)

type jsonPayload = map[string]any

var (
	jsonDoc    = momo.NewColumn[types.JSON[jsonPayload]]("events", "after", 0)
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
		{"pg text eq", dialect.Postgres, momo.JSONText(jsonDoc, "via").Eq("api"),
			`("events"."after" ->> 'via') = $1`, []any{"api"}},
		{"pg numeric key is a key", dialect.Postgres, momo.JSONText(jsonDoc, "0").Eq("x"),
			`("events"."after" ->> '0') = $1`, []any{"x"}},
		{"pg key with dot is bound", dialect.Postgres, momo.JSONText(jsonDoc, "a.b").Eq("x"),
			`("events"."after" ->> $1) = $2`, []any{"a.b", "x"}},
		{"pg text on nullable", dialect.Postgres, momo.JSONText(jsonBefore, "a'b").Ne("x"),
			`("events"."before" ->> $1) <> $2`, []any{"a'b", "x"}},
		{"pg text in", dialect.Postgres, momo.JSONText(jsonDoc, "via").In("a", "b"),
			`("events"."after" ->> 'via') IN ($1, $2)`, []any{"a", "b"}},
		{"pg text is null", dialect.Postgres, momo.JSONText(jsonDoc, "via").IsNull(),
			`("events"."after" ->> 'via') IS NULL`, nil},
		{"pg path", dialect.Postgres, momo.JSONPath(jsonDoc, "a", "0", "b").Eq("x"),
			`("events"."after" #>> '{a,0,b}') = $1`, []any{"x"}},
		{"pg path unsafe element is bound", dialect.Postgres, momo.JSONPath(jsonDoc, "a", "b,c").Eq("x"),
			`("events"."after" #>> $1) = $2`, []any{[]string{"a", "b,c"}, "x"}},
		{"pg contains", dialect.Postgres, momo.JSONContains(jsonDoc, map[string]any{"via": "api"}),
			`("events"."after" @> CAST($1 AS jsonb))`, []any{`{"via":"api"}`}},
		{"pg has key", dialect.Postgres, momo.JSONHasKey(jsonDoc, "via"),
			`("events"."after" ? $1)`, []any{"via"}},
		{"sqlite text", dialect.SQLite, momo.JSONText(jsonDoc, "via").Eq("api"),
			`CAST(json_extract("events"."after", '$."via"') AS TEXT) = ?`, []any{"api"}},
		{"sqlite numeric key stays an object key", dialect.SQLite, momo.JSONText(jsonDoc, "0").Eq("x"),
			`CAST(json_extract("events"."after", '$."0"') AS TEXT) = ?`, []any{"x"}},
		{"sqlite has numeric key is an object key", dialect.SQLite, momo.JSONHasKey(jsonDoc, "0"),
			`(json_type("events"."after", ?) IS NOT NULL)`, []any{`$."0"`}},
		{"sqlite path", dialect.SQLite, momo.JSONPath(jsonDoc, "a", "0", "b").Eq("x"),
			`CAST(json_extract("events"."after", '$."a"[0]."b"') AS TEXT) = ?`, []any{"x"}},
		{"sqlite has key", dialect.SQLite, momo.JSONHasKey(jsonDoc, "via"),
			`(json_type("events"."after", ?) IS NOT NULL)`, []any{`$."via"`}},
		{"mysql text", dialect.MySQL, momo.JSONText(jsonDoc, "via").Eq("api"),
			"JSON_UNQUOTE(JSON_EXTRACT(`events`.`after`, '$.\"via\"')) = ?", []any{"api"}},
		{"mysql contains", dialect.MySQL, momo.JSONContains(jsonDoc, []string{"a"}),
			"JSON_CONTAINS(`events`.`after`, CAST(? AS JSON))", []any{`["a"]`}},
		{"mysql has key", dialect.MySQL, momo.JSONHasKey(jsonDoc, "via"),
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
		if !(len(got.Args) == 0 && len(c.args) == 0) && !reflect.DeepEqual(got.Args, c.args) {
			t.Errorf("%s: args = %#v, want %#v", c.name, got.Args, c.args)
		}
	}
}

// Catches: the extraction unusable in ORDER BY or the select list.
func TestJSONTextOrderAndColumns(t *testing.T) {
	e := momo.JSONText(jsonDoc, "via")
	got, err := momo.Select[unitTally](typedDB(dialect.Postgres)).From(momo.TableRef{Name: "events"}).
		Columns(e).OrderBy(e.Desc()).SQL()
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT ("events"."after" ->> 'via') FROM "events" ORDER BY ("events"."after" ->> 'via') DESC`
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
		{"empty key", dialect.Postgres, momo.JSONText(jsonDoc, "").Eq("x"), momo.ErrValidation},
		{"empty path", dialect.Postgres, momo.JSONPath(jsonDoc).Eq("x"), momo.ErrValidation},
		{"quote on sqlite", dialect.SQLite, momo.JSONText(jsonDoc, `a"b`).Eq("x"), momo.ErrValidation},
		{"quote on postgres is fine", dialect.Postgres, momo.JSONText(jsonDoc, `a"b`).Eq("x"), nil},
		{"control char on mysql", dialect.MySQL, momo.JSONText(jsonDoc, "a\nb").Eq("x"), momo.ErrValidation},
		{"control char on sqlite", dialect.SQLite, momo.JSONHasKey(jsonDoc, "a\x01b"), momo.ErrValidation},
		{"backslash on sqlite", dialect.SQLite, momo.JSONText(jsonDoc, `a\b`).Eq("x"), momo.ErrValidation},
		{"backslash on mysql is escaped", dialect.MySQL, momo.JSONText(jsonDoc, `a\b`).Eq("x"), nil},
		{"has key empty", dialect.Postgres, momo.JSONHasKey(jsonDoc, ""), momo.ErrValidation},
		{"contains on sqlite", dialect.SQLite, momo.JSONContains(jsonDoc, jsonPayload{"a": 1}), momo.ErrUnsupported},
		{"unencodable value", dialect.Postgres, momo.JSONContains(jsonDoc, make(chan int)), momo.ErrValidation},
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

// Catches: a backslash in a MySQL key left unescaped, and a numeric
// JSONPath element not turned into an array index.
func TestJSONPathEscapingAndIndexes(t *testing.T) {
	got, err := jsonWhere(dialect.MySQL, momo.JSONText(jsonDoc, `a\b`).Eq("x"))
	if err != nil {
		t.Fatal(err)
	}
	if want := []any{`$."a\\b"`, "x"}; !reflect.DeepEqual(got.Args, want) {
		t.Errorf("mysql args = %#v, want %#v", got.Args, want)
	}
	got, err = jsonWhere(dialect.MySQL, momo.JSONPath(jsonDoc, "a", "1").Eq("x"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "JSON_UNQUOTE(JSON_EXTRACT(`events`.`after`, '$.\"a\"[1]')) = ?"; !strings.HasSuffix(got.SQL, want) {
		t.Errorf("mysql path = %s", got.SQL)
	}
}

// Catches: a value, key or path bound from a sensitive JSON column reaching
// hooks in the clear, and a non-sensitive column being masked.
func TestJSONOperatorsMaskSensitiveColumn(t *testing.T) {
	secret := "hunter2json" // plain-shaped, so only sensitivity keeps it out of the SQL text
	preds := map[string]momo.Predicate{
		"text eq":    momo.JSONText(jsonAfter, "via").Eq(secret),
		"plain key":  momo.JSONText(jsonAfter, secret).IsNull(),
		"plain path": momo.JSONPath(jsonAfter, secret, "b").IsNull(),
		"unsafe key": momo.JSONText(jsonAfter, secret+".x").IsNull(),
		"path":       momo.JSONPath(jsonAfter, "a", secret+".x").IsNull(),
		"contains":   momo.JSONContains(jsonAfter, map[string]any{"k": secret}),
		"has key":    momo.JSONHasKey(jsonAfter, secret),
		"in":         momo.JSONText(jsonAfter, "via").In(secret, "b"),
	}
	// A wrapper without Flags still masks, because rendering the column does.
	preds["through Coalesce"] = momo.JSONText(momo.Coalesce(jsonAfter, types.JSON[jsonPayload]{}), secret).Eq(secret)
	for name, p := range preds {
		rec := &obstest.Recorder{}
		db := momo.New(fakedriver.New(dialect.Postgres), momo.WithHook(rec), momo.WithArgLogging())
		_, _ = momo.Select[unitTally](db).From(momo.TableRef{Name: "events"}).Columns(momo.CountAll()).Where(p).All(t.Context())
		seen := false
		for _, e := range rec.Events() {
			seen = seen || len(e.Args) > 0
			if strings.Contains(e.SQL, secret) {
				t.Errorf("%s: statement text holds the secret: %s", name, e.SQL)
			}
			for _, a := range e.Args {
				if s, ok := a.(string); ok && strings.Contains(s, secret) {
					t.Errorf("%s: hook saw %q", name, s)
				}
				if ss, ok := a.([]string); ok && strings.Contains(strings.Join(ss, ","), secret) {
					t.Errorf("%s: hook saw %v", name, ss)
				}
			}
		}
		if !seen {
			t.Errorf("%s: no hook event with arguments", name)
		}
	}

	rec := &obstest.Recorder{}
	db := momo.New(fakedriver.New(dialect.Postgres), momo.WithHook(rec), momo.WithArgLogging())
	_, _ = momo.Select[unitTally](db).From(momo.TableRef{Name: "events"}).Columns(momo.CountAll()).
		Where(momo.JSONText(jsonBefore, "via").Eq("visible")).All(t.Context())
	for _, e := range rec.Events() {
		if len(e.Args) > 0 && !slices.Contains(e.Args, any("visible")) {
			t.Errorf("non-sensitive column was masked: %v", e.Args)
		}
	}
}
