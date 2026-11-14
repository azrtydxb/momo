// SPDX-License-Identifier: Apache-2.0

package testopen

import (
	"slices"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

// Catches: EachWithMySQL dropping one of the three servers CI runs, or Each
// growing MySQL entries and so changing every test that uses it.
func TestEachWithMySQLAddsEveryServer(t *testing.T) {
	var base, all []string
	for _, o := range openers {
		base = append(base, o.sub)
	}
	for _, o := range openersWithMySQL {
		all = append(all, o.sub)
		if o.sub != "postgres" && o.sub != "sqlite" && o.name != dialect.MySQL {
			t.Errorf("%s has dialect %q, want %q", o.sub, o.name, dialect.MySQL)
		}
	}
	if want := []string{"postgres", "sqlite"}; !slices.Equal(base, want) {
		t.Errorf("Each opens %v, want %v", base, want)
	}
	if want := []string{"postgres", "sqlite", "mysql84", "mysql97", "mariadb118"}; !slices.Equal(all, want) {
		t.Errorf("EachWithMySQL opens %v, want %v", all, want)
	}
}
