// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"slices"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

func checkInSample() *schema.Schema {
	return &schema.Schema{
		FormatVersion: schema.FormatVersion,
		Dialect:       dialect.Postgres,
		Tables: []schema.Table{{
			Name: "notes",
			Columns: []schema.Column{
				{Name: "id", Type: "bigint", Identity: "by_default"},
				{Name: "kind", Type: "text"},
				{Name: "label", Type: "text"},
				{Name: "qty", Type: "integer"},
			},
			PrimaryKey: []string{"id"},
			Checks: []schema.Check{
				{Name: "notes_kind_check", Expr: "(kind = ANY (ARRAY['draft'::text, 'published'::text, 'archived'::text]))"},
				{Name: "notes_label_in", Expr: "label IN ('y','x')"},
				{Name: "notes_qty_in", Expr: "qty IN ('1','2')"},
				{Name: "notes_qty_check", Expr: "qty > 0"},
			},
		}},
	}
}

// Catches: a value-list check written as one long string, values left in the
// order the database reports them, a default name spelled out or a custom one
// dropped, a list over a column that is not text turned into a declaration,
// and a snapshot that lacks the value list the models build.
func TestGenerateFromDBCheckIn(t *testing.T) {
	res, files := generate(t, checkInSample(), Config{})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	notes := files["notes"]
	mustMatch(t, "notes", notes,
		`c\.CheckIn\(&m\.Kind,\n\t\t"archived",\n\t\t"draft",\n\t\t"published",\n\t\)\n`,
		`c\.CheckIn\(&m\.Label,\n\t\t"x",\n\t\t"y",\n\t\)\.Name\("notes_label_in"\)`,
	)
	mustContain(t, "notes", notes,
		`c.Check("notes_qty_in", "qty IN ('1','2')")`,
		`c.Check("notes_qty_check", "qty > 0")`,
	)
	var kind *schema.Check
	for i, c := range res.Snapshot.Tables[0].Checks {
		if c.Name == "notes_kind_check" {
			kind = &res.Snapshot.Tables[0].Checks[i]
		}
	}
	if kind == nil || kind.In == nil || kind.In.Column != "kind" || !slices.Equal(kind.In.Values, []string{"archived", "draft", "published"}) {
		t.Fatalf("the snapshot check = %+v, want the sorted value list", kind)
	}
}

// Catches: a value list over an enum column or a column that the models type
// as something else being declared with CheckIn, which the schema build refuses.
func TestGenerateFromDBCheckInLeavesOtherColumnsAlone(t *testing.T) {
	s := checkInSample()
	s.Enums = []schema.Enum{{Name: "kinds", Values: []string{"a"}}}
	s.Tables[0].Columns[1] = schema.Column{Name: "kind", Type: "kinds", Enum: "kinds"}
	_, files := generate(t, s, Config{})
	mustContain(t, "notes", files["notes"], `c.Check("notes_kind_check", `)
}
