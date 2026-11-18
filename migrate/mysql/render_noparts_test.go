// SPDX-License-Identifier: Apache-2.0

package mysql_test

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/migrate"
	mmysql "github.com/azrtydxb/momo/migrate/mysql"
	"github.com/azrtydxb/momo/schema"
)

// TestRenderRefusesAnIndexWithNoParts: an index with no parts would render
// as CREATE INDEX name ON t (), which the server rejects; the renderer must
// refuse it up front, as the SQLite renderer does.
func TestRenderRefusesAnIndexWithNoParts(t *testing.T) {
	for name, ix := range map[string]schema.Index{
		"plain":    {Name: "ix_empty"},
		"unique":   {Name: "ix_empty", Unique: true},
		"fulltext": {Name: "ix_empty", Method: "fulltext"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := mmysql.New().Render([]migrate.Change{{Kind: migrate.CreateIndex, Table: "docs", Name: ix.Name, After: ix}})
			if err == nil {
				t.Fatal("render accepted an index with no parts")
			}
			if !strings.Contains(err.Error(), "has no parts") {
				t.Errorf("error = %q, want it to say the index has no parts", err)
			}
		})
	}
}
