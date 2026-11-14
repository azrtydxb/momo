// SPDX-License-Identifier: Apache-2.0

package jsonop_test

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/exprtest"
	"github.com/azrtydxb/momo/jsonop"
)

// Catches: TypeOf rendering a dialect's own type names (integer, text, TRUE,
// DOUBLE) instead of the six jsonb_typeof names, or a dialect missing from the
// mapping.
func TestTypeOfGolden(t *testing.T) {
	for _, c := range []struct {
		name string
		d    dialect.Name
		want string
	}{
		{"pg", dialect.Postgres, `SELECT jsonb_typeof("events"."after") FROM "events"`},
		{"sqlite", dialect.SQLite, `SELECT CASE json_type("events"."after") WHEN 'object' THEN 'object' WHEN 'array' THEN 'array'` +
			` WHEN 'text' THEN 'string' WHEN 'integer' THEN 'number' WHEN 'real' THEN 'number'` +
			` WHEN 'true' THEN 'boolean' WHEN 'false' THEN 'boolean' WHEN 'null' THEN 'null' END FROM "events"`},
		{"mysql", dialect.MySQL, "SELECT CASE JSON_TYPE(`events`.`after`) WHEN 'OBJECT' THEN 'object' WHEN 'ARRAY' THEN 'array'" +
			" WHEN 'STRING' THEN 'string' WHEN 'INTEGER' THEN 'number' WHEN 'UNSIGNED INTEGER' THEN 'number'" +
			" WHEN 'DOUBLE' THEN 'number' WHEN 'DECIMAL' THEN 'number'" +
			" WHEN 'BOOLEAN' THEN 'boolean' WHEN 'NULL' THEN 'null' END FROM `events`"},
	} {
		got, err := momo.Select[exprtest.Tally](exprtest.DB(c.d)).From(momo.TableRef{Name: "events"}).
			Columns(jsonop.TypeOf(jsonDoc)).SQL()
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got.SQL != c.want || len(got.Args) != 0 {
			t.Errorf("%s:\n got %s %v\nwant %s", c.name, got.SQL, got.Args, c.want)
		}
	}
}

// Catches: TypeOf losing the Eq comparison of a string expression, or binding
// the compared name wrongly.
func TestTypeOfComparison(t *testing.T) {
	got, err := jsonWhere(dialect.Postgres, jsonop.TypeOf(jsonBefore).Eq("number"))
	want := `jsonb_typeof("events"."before") = $1`
	if err != nil || len(got.Args) != 1 || got.Args[0] != "number" || !strings.Contains(got.SQL, want) {
		t.Errorf("got %s %v (%v), want ... %s", got.SQL, got.Args, err, want)
	}
}

// Catches: the sensitive flag of the column dropped by TypeOf, and a result
// that is not marked nullable (TypeOf is NULL for a SQL NULL column).
func TestTypeOfFlags(t *testing.T) {
	if fl := jsonop.TypeOf(jsonAfter).Flags(); fl&momo.FlagSensitive == 0 || fl&momo.FlagNullable == 0 {
		t.Errorf("flags of TypeOf(sensitive column) = %b, want sensitive and nullable", fl)
	}
	if fl := jsonop.TypeOf(jsonDoc).Flags(); fl&momo.FlagSensitive != 0 || fl&momo.FlagNullable == 0 {
		t.Errorf("flags of TypeOf(plain column) = %b, want nullable only", fl)
	}
}
