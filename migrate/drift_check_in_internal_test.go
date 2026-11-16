// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"testing"

	"github.com/azrtydxb/momo/schema"
)

func driftCheckIn(expr string, in *schema.CheckIn) *schema.Schema {
	return &schema.Schema{Tables: []schema.Table{{
		Name:    "notes",
		Columns: []schema.Column{{Name: "kind", Type: "text"}},
		Checks:  []schema.Check{{Name: "notes_kind_check", Expr: expr, In: in}},
	}}}
}

// Catches: a snapshot whose expression is spelled for another dialect than the
// database it runs on (double-quoted column on MySQL), or a list in another
// order, reported as drift although both hold the same set of values; and a
// live list with another value hidden by the same rule.
func TestCompareChecksValueListsAsSets(t *testing.T) {
	snapshot := driftCheckIn(`"kind" IN ('a','b')`, &schema.CheckIn{Column: "kind", Values: []string{"a", "b"}})
	same := map[string]*schema.Schema{
		"mysql spelling":   driftCheckIn("(`kind` in (_utf8mb4'b',_utf8mb4'a'))", nil),
		"postgres reading": driftCheckIn("(kind = ANY (ARRAY['b'::text, 'a'::text]))", nil),
		"both carry lists": driftCheckIn("kind in ('b','a')", &schema.CheckIn{Column: "kind", Values: []string{"b", "a"}}),
	}
	for name, live := range same {
		if rep := compare(snapshot, live, false, nil); !rep.Clean() {
			t.Errorf("%s: drift = %+v, want none", name, rep)
		}
	}
	other := driftCheckIn("(`kind` in (_utf8mb4'a',_utf8mb4'c'))", nil)
	if rep := compare(snapshot, other, false, nil); rep.Clean() {
		t.Error("another set of values must be drift")
	}
	if snapshot.Tables[0].Checks[0].Expr != `"kind" IN ('a','b')` {
		t.Error("compare changed the caller's snapshot")
	}
}
