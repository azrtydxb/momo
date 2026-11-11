// SPDX-License-Identifier: Apache-2.0

package jsonop_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/exprtest"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/jsonop"
	"github.com/azrtydxb/momo/types"
)

func buildObject() momo.Expression[types.JSON[jsonPayload]] {
	return momo.Func[types.JSON[jsonPayload]]("jsonb_build_object", "k", "v")
}

func setPath() momo.Expression[[]string] { return momo.ArrayOf[string](momo.Value("at_k")) }

func setValue() momo.Expression[types.JSON[int]] { return momo.Func[types.JSON[int]]("to_jsonb", 5) }

// Catches: Root rendering a path operator that rejects the empty path, an
// extraction that is not the dialect's scalar read, a delete or concat or set
// that renders the wrong operator or interpolates its key, and Concat or Set
// rendering a lookalike on a dialect whose operator has other semantics.
func TestJSONEditOperatorsGolden(t *testing.T) {
	cases := []struct {
		name string
		d    dialect.Name
		p    momo.Predicate
		want string
		args []any
	}{
		{"pg root", dialect.Postgres, jsonop.Root(jsonDoc).Eq("5"),
			`("events"."after" #>> '{}') = $1`, []any{"5"}},
		{"pg root on nullable", dialect.Postgres, jsonop.Root(jsonBefore).IsNull(),
			`("events"."before" #>> '{}') IS NULL`, nil},
		{"sqlite root", dialect.SQLite, jsonop.Root(jsonDoc).Eq("5"),
			`CAST(json_extract("events"."after", '$') AS TEXT) = ?`, []any{"5"}},
		{"mysql root", dialect.MySQL, jsonop.Root(jsonDoc).Eq("5"),
			"JSON_UNQUOTE(JSON_EXTRACT(`events`.`after`, '$')) = ?", []any{"5"}},

		{"pg delete key", dialect.Postgres, jsonop.Text(jsonop.DeleteKey(jsonDoc, "password_hash"), "via").Eq("x"),
			`(("events"."after" - CAST($1 AS text)) ->> 'via') = $2`, []any{"password_hash", "x"}},
		{"pg delete key chained", dialect.Postgres, jsonop.HasKey(jsonop.DeleteKey(jsonop.DeleteKey(jsonDoc, "a"), "b"), "c"),
			`((("events"."after" - CAST($1 AS text)) - CAST($2 AS text)) ? $3)`, []any{"a", "b", "c"}},
		{"sqlite delete key", dialect.SQLite, jsonop.Text(jsonop.DeleteKey(jsonDoc, "password_hash"), "via").Eq("x"),
			`CAST(json_extract(json_remove("events"."after", ?), '$."via"') AS TEXT) = ?`, []any{`$."password_hash"`, "x"}},
		{"mysql delete key", dialect.MySQL, jsonop.Text(jsonop.DeleteKey(jsonDoc, "password_hash"), "via").Eq("x"),
			"JSON_UNQUOTE(JSON_EXTRACT(JSON_REMOVE(`events`.`after`, ?), '$.\"via\"')) = ?", []any{`$."password_hash"`, "x"}},
		{"pg delete key is bound as data", dialect.Postgres, jsonop.HasKey(jsonop.DeleteKey(jsonDoc, "a' OR '1'='1"), "via"),
			`(("events"."after" - CAST($1 AS text)) ? $2)`, []any{"a' OR '1'='1", "via"}},

		{"pg concat of columns", dialect.Postgres, jsonop.Text(jsonop.Concat(jsonDoc, jsonBefore), "via").Eq("x"),
			`(("events"."after" || "events"."before") ->> 'via') = $1`, []any{"x"}},
		{"pg concat of a call", dialect.Postgres, jsonop.Text(jsonop.Concat(jsonDoc, buildObject()), "k").Eq("v"),
			`(("events"."after" || jsonb_build_object($1, $2)) ->> 'k') = $3`, []any{"k", "v", "v"}},
		{"pg concat after delete", dialect.Postgres, jsonop.HasKey(jsonop.Concat(jsonop.DeleteKey(jsonDoc, "a"), buildObject()), "k"),
			`((("events"."after" - CAST($1 AS text)) || jsonb_build_object($2, $3)) ? $4)`, []any{"a", "k", "v", "k"}},

		{"pg set with a computed path", dialect.Postgres, jsonop.Text(jsonop.Set(jsonDoc, setPath(), setValue()), "at_k").Eq("5"),
			`(jsonb_set("events"."after", ARRAY[$1], to_jsonb($2)) ->> 'at_k') = $3`, []any{"at_k", 5, "5"}},
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
		if (len(got.Args) != 0 || len(c.args) != 0) && !reflect.DeepEqual(got.Args, c.args) {
			t.Errorf("%s: args = %#v, want %#v", c.name, got.Args, c.args)
		}
	}
}

// Catches: the computed JSON value unusable in the select list, and dropping
// the sensitive flag when a value is derived from a sensitive column.
func TestJSONEditColumnsAndFlags(t *testing.T) {
	e := jsonop.DeleteKey(jsonDoc, "password_hash")
	got, err := momo.Select[exprtest.Tally](exprtest.DB(dialect.Postgres)).From(momo.TableRef{Name: "events"}).
		Columns(e).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if want := `SELECT ("events"."after" - CAST($1 AS text)) FROM "events"`; got.SQL != want {
		t.Errorf("got  %s\nwant %s", got.SQL, want)
	}

	sens := map[string]momo.Expr{
		"delete key": jsonop.DeleteKey(jsonAfter, "k"),
		"concat":     jsonop.Concat(jsonAfter, buildObject()),
		"concat rhs": jsonop.Concat(jsonDoc, jsonAfter),
		"set":        jsonop.Set(jsonAfter, setPath(), setValue()),
	}
	for name, x := range sens {
		if momo.FlagsOf(x)&momo.FlagSensitive == 0 {
			t.Errorf("%s: lost the sensitive flag", name)
		}
	}
	if momo.FlagsOf(jsonop.DeleteKey(jsonDoc, "k"))&momo.FlagSensitive != 0 {
		t.Error("delete key on a non-sensitive column is marked sensitive")
	}
	if jsonop.Text(jsonop.DeleteKey(jsonAfter, "k"), "via").Flags()&momo.FlagSensitive == 0 {
		t.Error("Text over a derived sensitive value lost the sensitive flag")
	}
}

// Catches: an unrepresentable key reaching the database, and Concat or Set
// silently rendering a different operator on SQLite or MySQL instead of
// ErrUnsupported.
func TestJSONEditOperatorErrors(t *testing.T) {
	cases := []struct {
		name string
		d    dialect.Name
		p    momo.Predicate
		kind error
	}{
		{"delete empty key", dialect.Postgres, jsonop.HasKey(jsonop.DeleteKey(jsonDoc, ""), "a"), momo.ErrValidation},
		{"delete quote on sqlite", dialect.SQLite, jsonop.HasKey(jsonop.DeleteKey(jsonDoc, `a"b`), "a"), momo.ErrValidation},
		{"delete quote on postgres is fine", dialect.Postgres, jsonop.HasKey(jsonop.DeleteKey(jsonDoc, `a"b`), "a"), nil},
		{"delete on sqlite", dialect.SQLite, jsonop.HasKey(jsonop.DeleteKey(jsonDoc, "a"), "b"), nil},
		{"delete on mysql", dialect.MySQL, jsonop.HasKey(jsonop.DeleteKey(jsonDoc, "a"), "b"), nil},
		{"concat on sqlite", dialect.SQLite, jsonop.HasKey(jsonop.Concat(jsonDoc, jsonBefore), "a"), momo.ErrUnsupported},
		{"concat on mysql", dialect.MySQL, jsonop.HasKey(jsonop.Concat(jsonDoc, jsonBefore), "a"), momo.ErrUnsupported},
		{"set on sqlite", dialect.SQLite, jsonop.HasKey(jsonop.Set(jsonDoc, setPath(), setValue()), "a"), momo.ErrUnsupported},
		{"set on mysql", dialect.MySQL, jsonop.HasKey(jsonop.Set(jsonDoc, setPath(), setValue()), "a"), momo.ErrUnsupported},
		{"set with an array on sqlite", dialect.SQLite, jsonop.HasKey(jsonop.Set(jsonDoc, momo.ArrayOf[string](momo.Value("a")), setValue()), "a"), momo.ErrUnsupported},
		{"path with no elements is still refused", dialect.Postgres, jsonop.Path(jsonDoc).Eq("x"), momo.ErrValidation},
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

// Catches: a key, path element or value bound through DeleteKey, Concat or
// Set from a sensitive JSON column reaching hooks in the clear.
func TestJSONEditOperatorsMaskSensitiveColumn(t *testing.T) {
	secret := "hunter2edit"
	secretValue := momo.Func[types.JSON[int]]("to_jsonb", secret)
	secretObject := momo.Func[types.JSON[jsonPayload]]("jsonb_build_object", "k", secret)
	preds := map[string]momo.Predicate{
		"delete key": jsonop.HasKey(jsonop.DeleteKey(jsonAfter, secret), "a"),
		"concat":     jsonop.HasKey(jsonop.Concat(jsonAfter, secretObject), "a"),
		"set path":   jsonop.HasKey(jsonop.Set(jsonAfter, momo.ArrayOf[string](momo.Value(secret)), setValue()), "a"),
		"set value":  jsonop.HasKey(jsonop.Set(jsonAfter, setPath(), secretValue), "a"),
	}
	for name, p := range preds {
		rec := &obstest.Recorder{}
		db := momo.New(fakedriver.New(dialect.Postgres), momo.WithHook(rec), momo.WithArgLogging())
		_, _ = momo.Select[exprtest.Tally](db).From(momo.TableRef{Name: "events"}).Columns(momo.CountAll()).Where(p).All(t.Context())
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
			}
		}
		if !seen {
			t.Errorf("%s: no hook event with arguments", name)
		}
	}
}
