// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"testing"

	"github.com/azrtydxb/momo/schema"
)

// Catches: introspect.only leaving the tables outside the list in the managed
// schema, so drift and baseline report every table the project has not moved
// yet as unmanaged.
func TestApplyOnlyHidesTablesOutsideTheList(t *testing.T) {
	s := &schema.Schema{
		Tables: []schema.Table{{Name: "accounts"}, {Name: "projects"}, {Schema: "app", Name: "tickets"}},
		Triggers: []schema.Trigger{
			{Table: "accounts", Name: "accounts_touch"},
			{Table: "projects", Name: "projects_touch"},
		},
		Policies: []schema.Policy{{Table: "projects", Name: "projects_rls"}},
	}
	got := applyOnly(s, []string{"accounts", "app.*"})
	var names []string
	for _, tb := range got.Tables {
		names = append(names, tb.Name)
	}
	if len(names) != 2 || names[0] != "accounts" || names[1] != "tickets" {
		t.Fatalf("tables = %v, want accounts and tickets", names)
	}
	if len(got.Triggers) != 1 || got.Triggers[0].Table != "accounts" {
		t.Fatalf("triggers = %+v, want only the accounts trigger", got.Triggers)
	}
	if len(got.Policies) != 0 {
		t.Fatalf("policies = %+v, want none", got.Policies)
	}
	if len(s.Tables) != 3 {
		t.Fatal("applyOnly modified its input")
	}
	if same := applyOnly(s, nil); len(same.Tables) != 3 {
		t.Fatal("an empty list must keep every table")
	}
}
