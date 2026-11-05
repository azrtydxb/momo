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
			if orm == "ent" && field == "HTTPURL" {
				// Ent's column here is the schema field's own name
				// ("http_url"), not derived from the Go name; the word
				// list below records ent's actual Go-name rule.
				continue
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

// TestNamingPresetsWords checks every word of the recorded word list: the
// table and column each preset produces must equal what the ORM's own naming
// functions returned. It catches a wrong initialism list, a missing plural
// rule, an uncountable or irregular word, and a snake-case boundary rule.
func TestNamingPresetsWords(t *testing.T) {
	for _, orm := range []string{"gorm", "bun", "ent"} {
		b, err := os.ReadFile("testdata/" + orm + ".json")
		if err != nil {
			t.Fatal(err)
		}
		var f fixture
		if err := json.Unmarshal(b, &f); err != nil {
			t.Fatal(err)
		}
		if len(f.Words.Tables) < 30 || len(f.Words.Columns) != len(f.Words.Tables) {
			t.Fatalf("%s: word list too small: %d tables, %d columns", orm, len(f.Words.Tables), len(f.Words.Columns))
		}
		n, err := ByName(orm)
		if err != nil {
			t.Fatal(err)
		}
		for word, want := range f.Words.Tables {
			if got := n.TableName(word); got != want {
				t.Errorf("%s TableName(%s)=%q want %q", orm, word, got, want)
			}
		}
		for word, want := range f.Words.Columns {
			if got := n.ColumnName(word); got != want {
				t.Errorf("%s ColumnName(%s)=%q want %q", orm, word, got, want)
			}
		}
	}
}
