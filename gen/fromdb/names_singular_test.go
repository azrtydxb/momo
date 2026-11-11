// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"testing"

	"github.com/azrtydxb/momo/gen/naming"
)

// TestStructNameSingularWords pins the model name --from-db derives from a
// table name, under every naming preset naming.ByName knows. Group A are the
// words ending in -se (the -s strip is right, the -es strip gives "Releas");
// group B take -es (the -s strip gives "Addresse"); the rest are -ies and
// irregular plurals.
func TestStructNameSingularWords(t *testing.T) {
	presets := []string{"momo", "gorm", "bun", "ent", "prisma"}
	cases := []struct {
		table, want string
		// cannot lists presets whose pluralizer cannot give the table name
		// back from the expected model name, with the reason. The result for
		// such a preset depends on which wrong candidate happens to round-trip,
		// so it is not pinned.
		cannot map[string]string
	}{
		// Group A: -se words.
		{"releases", "Release", nil},
		{"cases", "Case", nil},
		{"courses", "Course", nil},
		{"responses", "Response", nil},
		{"licenses", "License", nil},
		{"bases", "Base", nil},
		{"houses", "House", nil},
		{"purposes", "Purpose", nil},
		{"uses", "Use", nil},
		// Group B: -es words.
		{"addresses", "Address", nil},
		{"classes", "Class", nil},
		{"statuses", "Status", map[string]string{
			"ent": "ent matches its status rule case-sensitively, so Status is not pluralized to statuses",
		}},
		{"buses", "Bus", map[string]string{
			"ent": "ent matches its bus rule case-sensitively, so Bus is not pluralized to buses",
		}},
		{"boxes", "Box", nil},
		{"matches", "Match", nil},
		{"wishes", "Wish", nil},
		{"quizzes", "Quiz", map[string]string{
			"momo": "momo pluralizes quiz to quizes",
			"ent":  "ent pluralizes only the lower-case word quiz to quizzes, not Quiz",
		}},
		// -ies and irregular plurals.
		{"categories", "Category", nil},
		{"companies", "Company", nil},
		{"users", "User", nil},
		{"order_items", "OrderItem", nil},
		{"people", "Person", nil},
	}
	for _, name := range presets {
		namer, err := naming.ByName(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cases {
			if _, skip := c.cannot[name]; skip {
				continue
			}
			g := &generator{o: Options{Namer: namer}}
			if got := g.structName(c.table); got != c.want {
				t.Errorf("%s: structName(%q) = %q, want %q", name, c.table, got, c.want)
			}
		}
	}
}
