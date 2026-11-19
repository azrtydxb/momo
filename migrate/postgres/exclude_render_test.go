// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

func excludeTables(mutate func(*schema.Exclude)) *schema.Schema {
	x := schema.Exclude{
		Name:     "bookings_no_overlap",
		Elements: []schema.ExcludeElement{{Column: "room_id", Operator: "="}, {Column: "during", Operator: "&&"}},
	}
	if mutate != nil {
		mutate(&x)
	}
	return &schema.Schema{
		Dialect:    "postgres",
		Extensions: []schema.Extension{{Name: "btree_gist"}},
		Tables: []schema.Table{{
			Name: "bookings",
			Columns: []schema.Column{
				{Name: "id", Type: "bigint"},
				{Name: "room_id", Type: "integer"},
				{Name: "during", Type: "tstzrange"},
			},
			PrimaryKey: []string{"id"},
			Excludes:   []schema.Exclude{x},
		}},
	}
}

// Catches: an exclusion constraint left out of CREATE TABLE, written with the
// wrong clause order, or created before the extension its operator class comes
// from: btree_gist must exist before the table that needs it.
func TestRenderExcludeConstraintInCreateTable(t *testing.T) {
	sql := renderDiff(t, &schema.Schema{Dialect: "postgres"}, excludeTables(nil))
	want := `CONSTRAINT "bookings_no_overlap" EXCLUDE USING gist ("room_id" WITH =, "during" WITH &&)`
	if !strings.Contains(sql, want) {
		t.Errorf("CREATE TABLE lacks %s:\n%s", want, sql)
	}
	ext, tbl := strings.Index(sql, "CREATE EXTENSION"), strings.Index(sql, "CREATE TABLE")
	if ext < 0 || tbl < 0 || ext > tbl {
		t.Errorf("the extension is not created before the table:\n%s", sql)
	}
	if strings.Contains(sql, "DEFERRABLE") {
		t.Errorf("a plain constraint renders a deferrability clause:\n%s", sql)
	}
}

// Catches: the deferral flags and the access method of an exclusion constraint
// lost in the DDL, and the add and drop of one on an existing table written as
// anything but a constraint statement.
func TestRenderExcludeConstraintAlterAndOptions(t *testing.T) {
	bare := excludeTables(nil)
	bare.Tables[0].Excludes = nil

	add := renderDiff(t, bare, excludeTables(func(x *schema.Exclude) { x.Deferrable, x.InitiallyDeferred = true, true }))
	want := `ALTER TABLE "bookings" ADD CONSTRAINT "bookings_no_overlap" EXCLUDE USING gist ("room_id" WITH =, "during" WITH &&) DEFERRABLE INITIALLY DEFERRED`
	if !strings.Contains(add, want) {
		t.Errorf("the add lacks %s:\n%s", want, add)
	}

	spgist := renderDiff(t, bare, excludeTables(func(x *schema.Exclude) {
		x.Method = "spgist"
		x.Deferrable = true
	}))
	if !strings.Contains(spgist, `EXCLUDE USING spgist (`) || !strings.Contains(spgist, ") DEFERRABLE") || strings.Contains(spgist, "INITIALLY") {
		t.Errorf("the method or the deferral is wrong:\n%s", spgist)
	}

	drop := renderDiff(t, excludeTables(nil), bare)
	if !strings.Contains(drop, `ALTER TABLE "bookings" DROP CONSTRAINT "bookings_no_overlap"`) {
		t.Errorf("the drop is wrong:\n%s", drop)
	}

	replace := renderDiff(t, excludeTables(nil), excludeTables(func(x *schema.Exclude) { x.Elements[1].Operator = "-|-" }))
	if i, j := strings.Index(replace, "DROP CONSTRAINT"), strings.Index(replace, "ADD CONSTRAINT"); i < 0 || j < 0 || i > j {
		t.Errorf("the replace does not drop before it adds:\n%s", replace)
	}
}

// Catches: an operator or an access method reaching the statement unquoted
// without being checked, which would let a hostile model write SQL, and a
// constraint with no element.
func TestRenderExcludeConstraintRefusesHostileParts(t *testing.T) {
	for name, mutate := range map[string]func(*schema.Exclude){
		"operator":    func(x *schema.Exclude) { x.Elements[0].Operator = "=; DROP TABLE bookings; --" },
		"no operator": func(x *schema.Exclude) { x.Elements[0].Operator = "" },
		"method":      func(x *schema.Exclude) { x.Method = "gist; DROP TABLE bookings" },
		"no elements": func(x *schema.Exclude) { x.Elements = nil },
		"no column":   func(x *schema.Exclude) { x.Elements[0].Column = "" },
	} {
		changes, _, err := migrate.Diff(&schema.Schema{Dialect: "postgres"}, excludeTables(mutate), migrate.DiffOptions{})
		if err != nil {
			t.Fatalf("%s: diff: %v", name, err)
		}
		ordered, err := migrate.OrderWithViews(changes, nil, nil)
		if err != nil {
			t.Fatalf("%s: order: %v", name, err)
		}
		if _, err := (mpg.Dialect{}).Render(ordered); err == nil {
			t.Errorf("%s: the hostile constraint was rendered", name)
		}
	}
}
