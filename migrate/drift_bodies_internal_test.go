// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"slices"
	"testing"

	"github.com/azrtydxb/momo/schema"
)

func bodySchema(view, when, using, check string) *schema.Schema {
	return &schema.Schema{
		Dialect: "postgres",
		Tables:  []schema.Table{{Name: "docs", Columns: []schema.Column{{Name: "id", Type: "bigint"}}}},
		Views:   []schema.View{{Name: "v_docs", Query: view}},
		Triggers: []schema.Trigger{{
			Table: "docs", Name: "docs_touch", Timing: "BEFORE", Events: []string{"INSERT"}, ForEachRow: true,
			When: when, Function: "touch()",
		}},
		Policies: []schema.Policy{{Table: "docs", Name: "docs_pos", Command: "ALL", Permissive: true, Using: using, Check: check}},
	}
}

// Catches: a body comparison that is switched off (the old behaviour) reports a
// policy loosened out of band as Clean.
func TestCompareExactReportsBodyChanges(t *testing.T) {
	snap := bodySchema("SELECT id FROM docs", "(new.id > 0)", "(id > 0)", "(id > 0)")
	live := bodySchema("SELECT id FROM docs WHERE true", "(new.id > 1)", "true", "(id > 5)")

	exact := compare(snap, live, true, nil)
	got := map[string]string{}
	for _, c := range exact.Changed {
		got[c.Object.Kind] = c.Detail
	}
	if got["view"] != "query differ" || got["trigger"] != "when differ" || got["policy"] != "check, using differ" {
		t.Fatalf("exact comparison must name the changed bodies, got %v", got)
	}

	loose := compare(snap, live, false, nil)
	if !loose.Clean() {
		t.Fatalf("a non-exact comparison ignores bodies: %+v", loose)
	}
	var un []string
	for _, r := range unverifiedBodies(snap, live) {
		un = append(un, r.Kind)
	}
	if !slices.Equal(un, []string{"policy", "trigger", "view"}) {
		t.Fatalf("unverified = %v", un)
	}

	same := compare(snap, bodySchema("SELECT id FROM docs", "(new.id > 0)", "(id > 0)", "(id > 0)"), true, nil)
	if !same.Clean() {
		t.Fatalf("equal bodies are clean: %+v", same)
	}
}

func TestHasBodyText(t *testing.T) {
	if hasBodyText(&schema.Schema{Tables: []schema.Table{{Name: "t"}}}) {
		t.Fatal("tables alone carry no body text")
	}
	for name, s := range map[string]*schema.Schema{
		"view":    {Views: []schema.View{{Name: "v", Query: "SELECT 1"}}},
		"trigger": {Triggers: []schema.Trigger{{Name: "t", When: "x"}}},
		"using":   {Policies: []schema.Policy{{Name: "p", Using: "true"}}},
		"check":   {Policies: []schema.Policy{{Name: "p", Check: "true"}}},
	} {
		if !hasBodyText(s) {
			t.Errorf("%s: want body text", name)
		}
	}
}
