// SPDX-License-Identifier: Apache-2.0

package dialect

import (
	"os"
	"strings"
	"testing"
)

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// Catches: docs/mysql.md claiming a capability the dialect does not have, or
// the reverse, after a flag changes.
func TestCapabilityMatrixMatchesDialect(t *testing.T) {
	raw, err := os.ReadFile("../docs/mysql.md")
	if err != nil {
		t.Fatal(err)
	}
	my := MySQLFlavor(FlavorMySQL)
	ma := MySQLFlavor(FlavorMariaDB)
	rows := map[string][2]bool{
		"Returning":        {my.Returning, ma.Returning},
		"InsertReturning":  {my.InsertReturning, ma.InsertReturning},
		"OnConflict":       {my.OnConflict, ma.OnConflict},
		"OnDuplicateKey":   {my.OnDuplicateKey, ma.OnDuplicateKey},
		"NullsOrdering":    {my.NullsOrdering, ma.NullsOrdering},
		"TransactionalDDL": {my.TransactionalDDL, ma.TransactionalDDL},
		"Merge":            {my.Merge, ma.Merge},
		"ILike":            {my.ILike, ma.ILike},
	}
	for name, v := range rows {
		found := false
		for _, line := range strings.Split(string(raw), "\n") {
			cells := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
			if len(cells) != 3 || strings.TrimSpace(cells[0]) != name {
				continue
			}
			found = true
			if got := strings.TrimSpace(cells[1]); got != yesNo(v[0]) {
				t.Errorf("docs/mysql.md %s mysql = %q, dialect says %s", name, got, yesNo(v[0]))
			}
			if got := strings.TrimSpace(cells[2]); got != yesNo(v[1]) {
				t.Errorf("docs/mysql.md %s mariadb = %q, dialect says %s", name, got, yesNo(v[1]))
			}
		}
		if !found {
			t.Errorf("docs/mysql.md lacks a matrix row for %s", name)
		}
	}
}
