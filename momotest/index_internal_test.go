// SPDX-License-Identifier: Apache-2.0

package momotest

import (
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func TestPlanUsesIndex(t *testing.T) {
	cases := []struct {
		name       string
		d          dialect.Name
		lines      []string
		index      string
		found, seq bool
	}{
		{"pg index scan", dialect.Postgres, []string{"Index Scan using docs_title_idx on docs  (cost=0.15..8.17 rows=1 width=4)", "  Index Cond: (title = $1)"}, "docs_title_idx", true, false},
		{"pg index only", dialect.Postgres, []string{"Index Only Scan using docs_title_idx on docs"}, "docs_title_idx", true, false},
		{"pg backward", dialect.Postgres, []string{"Index Scan Backward using docs_title_idx on docs"}, "docs_title_idx", true, false},
		{"pg bitmap", dialect.Postgres, []string{"Bitmap Heap Scan on docs", "  ->  Bitmap Index Scan on docs_title_idx"}, "docs_title_idx", true, false},
		{"pg seq scan", dialect.Postgres, []string{"Seq Scan on docs  (cost=0.00..1.04 rows=1 width=4)", "  Filter: (title = $1)"}, "docs_title_idx", false, true},
		{"pg longer name is not a match", dialect.Postgres, []string{"Index Scan using docs_title_idx2 on docs"}, "docs_title_idx", false, false},
		{"pg index and seq scan", dialect.Postgres, []string{"Nested Loop", "  ->  Seq Scan on a", "  ->  Index Scan using docs_title_idx on docs"}, "docs_title_idx", true, true},
		{"pg quoted mixed case", dialect.Postgres, []string{`Index Scan using "MyIdx" on docs  (cost=0.15..8.17 rows=1 width=4)`}, "MyIdx", true, false},
		{"pg quoted mixed case, other name", dialect.Postgres, []string{`Index Scan using "MyIdx2" on docs`}, "MyIdx", false, false},
		{"pg hyphen is not a boundary", dialect.Postgres, []string{`Index Scan using "docs-title-idx-2" on docs`}, "docs-title-idx", false, false},
		{"pg hyphenated name", dialect.Postgres, []string{`Index Scan using "docs-title-idx" on docs`}, "docs-title-idx", true, false},
		{"pg dot is not a boundary", dialect.Postgres, []string{`Index Scan using "docs.idx.old" on docs`}, "docs.idx", false, false},
		{"pg name at line end", dialect.Postgres, []string{`  ->  Bitmap Index Scan on docs_title_idx`}, "docs_title_idx", true, false},
		{"pg regexp metacharacters are literal", dialect.Postgres, []string{`Index Scan using "docsXtitle" on docs`}, "docs.title", false, false},
		{"sqlite index", dialect.SQLite, []string{"SEARCH items USING INDEX items_sku_idx (sku=?)"}, "items_sku_idx", true, false},
		{"sqlite covering", dialect.SQLite, []string{"SEARCH items USING COVERING INDEX items_sku_idx (sku=?)"}, "items_sku_idx", true, false},
		{"sqlite quoted mixed case", dialect.SQLite, []string{`SEARCH items USING INDEX "MyIdx" (sku=?)`}, "MyIdx", true, false},
		{"sqlite scan", dialect.SQLite, []string{"SCAN items"}, "items_sku_idx", false, true},
		{"sqlite longer name is not a match", dialect.SQLite, []string{"SEARCH items USING INDEX items_sku_idx2 (sku=?)"}, "items_sku_idx", false, false},
	}
	for _, c := range cases {
		found, seq := planUsesIndex(c.d, c.lines, c.index)
		if found != c.found || seq != c.seq {
			t.Errorf("%s: found=%v seq=%v, want found=%v seq=%v", c.name, found, seq, c.found, c.seq)
		}
	}
}
