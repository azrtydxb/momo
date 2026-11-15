// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/schema"
)

// Catches: a full-text declaration rendered as an ordinary index before the
// dialect creates the FTS5 table.
func TestCreateIndexSQLRefusesFullText(t *testing.T) {
	ix := schema.Index{Name: "articles_search", Method: "fulltext",
		Parts:    []schema.IndexPart{{Column: "title"}},
		FullText: &schema.FullText{Columns: []string{"title"}}}
	_, err := createIndexSQL("articles", ix)
	if !errors.Is(err, momo.ErrUnsupported) || !strings.Contains(err.Error(), "full-text declarations are not rendered yet") {
		t.Fatalf("err = %v, want ErrUnsupported naming the missing rendering", err)
	}
}
