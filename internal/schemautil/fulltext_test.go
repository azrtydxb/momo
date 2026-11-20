// SPDX-License-Identifier: Apache-2.0

package schemautil_test

import (
	"testing"

	"github.com/azrtydxb/momo/internal/schemautil"
)

// Catches: a drift in the names derived for the FTS5 table and the generated
// tsvector column, which every migration engine and the generator rely on.
func TestFullTextNames(t *testing.T) {
	if schemautil.FTS5Table("articles") != "articles_fts" || schemautil.TSVectorColumn("articles_search") != "articles_search_tsv" {
		t.Fatal("name functions drifted")
	}
}
