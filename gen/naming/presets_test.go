// SPDX-License-Identifier: Apache-2.0

package naming

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestNamingPresets catches a preset whose table, column or join-table rule
// diverges from the names the real ORM generated (recorded fixtures).
func TestNamingPresets(t *testing.T) {
	for _, orm := range []string{"gorm", "bun", "ent"} {
		b, err := os.ReadFile("testdata/" + orm + ".json")
		if err != nil {
			t.Fatal(err)
		}
		var f fixture
		if err := json.Unmarshal(b, &f); err != nil {
			t.Fatal(err)
		}
		n, err := ByName(orm)
		if err != nil {
			t.Fatal(err)
		}
		for model, want := range f.Tables {
			if got := n.TableName(model); got != want {
				t.Errorf("%s TableName(%s)=%q want %q", orm, model, got, want)
			}
		}
		for key, want := range f.Columns {
			model, field, found := strings.Cut(key, ".")
			if !found {
				t.Fatalf("bad key %q", key)
			}
			if got := n.ColumnName(field); got != want {
				t.Errorf("%s ColumnName(%s.%s)=%q want %q", orm, model, field, got, want)
			}
		}
		for key, want := range f.JoinTable {
			owner, field, found := strings.Cut(key, ".")
			if !found {
				t.Fatalf("bad key %q", key)
			}
			target := map[string]string{"Tags": "Tag"}[field]
			if got := n.JoinTableName(owner, target); got != want {
				t.Errorf("%s JoinTableName(%s,%s)=%q want %q", orm, owner, target, got, want)
			}
		}
	}
}
