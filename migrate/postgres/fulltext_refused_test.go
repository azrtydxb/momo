// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/schema"
)

// Catches: a full-text declaration rendered as the invalid
// CREATE INDEX ... USING "fulltext" before the dialect renders it properly.
func TestCreateIndexRefusesFullText(t *testing.T) {
	ix := schema.Index{Name: "articles_search", Method: "fulltext",
		Parts:    []schema.IndexPart{{Column: "title"}},
		FullText: &schema.FullText{Columns: []string{"title"}, Config: "english"}}
	_, err := createIndex("", "articles", ix)
	if !errors.Is(err, momo.ErrUnsupported) || !strings.Contains(err.Error(), "full-text declarations are not rendered yet") {
		t.Fatalf("err = %v, want ErrUnsupported naming the missing rendering", err)
	}
}
