// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"reflect"
	"testing"
)

// Catches: a non-FTS5 virtual table claimed as a full-text table, a lost
// tokenizer, a changed column order, and an FTS5 table that has no external
// content or an option the declaration cannot hold.
func TestParseFTS5(t *testing.T) {
	content, ft, ok := parseFTS5(`CREATE VIRTUAL TABLE "articles_fts" USING fts5("title", "body", content='articles', content_rowid='id', tokenize='porter unicode61')`)
	if !ok || content != "articles" || !reflect.DeepEqual(ft.Columns, []string{"title", "body"}) || ft.Tokenizer != "porter unicode61" {
		t.Fatalf("got %q %+v %v", content, ft, ok)
	}
	content, ft, ok = parseFTS5(`CREATE VIRTUAL TABLE articles_fts USING fts5(title, body, content = articles, content_rowid = id)`)
	if !ok || content != "articles" || ft.Tokenizer != "" || !reflect.DeepEqual(ft.Columns, []string{"title", "body"}) {
		t.Fatalf("bare form: %q %+v %v", content, ft, ok)
	}
	for _, sql := range []string{
		`CREATE VIRTUAL TABLE r USING rtree(id, minx, maxx)`,
		`CREATE VIRTUAL TABLE f USING fts5(a, b)`,
		`CREATE VIRTUAL TABLE f USING fts5(a, b, content='t', prefix='2')`,
		`CREATE VIRTUAL TABLE f USING fts5(a UNINDEXED, b, content='t')`,
		`CREATE TABLE t (a)`,
	} {
		if _, _, ok := parseFTS5(sql); ok {
			t.Errorf("accepted %s", sql)
		}
	}
}
