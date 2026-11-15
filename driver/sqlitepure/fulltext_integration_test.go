// SPDX-License-Identifier: Apache-2.0

//go:build integration

package sqlitepure

import (
	"context"
	"strings"
	"testing"
)

// Catches: a pure-Go SQLite build without FTS5 or bm25, reported as the raw
// driver string. The cgo driver runs the same probe in migrate/sqlite.
func TestFullTextSQLitePureDriverSupport(t *testing.T) {
	_, d := openPure(t, Config{})
	ctx := context.Background()
	for _, sql := range []string{`CREATE VIRTUAL TABLE p USING fts5(a)`, `INSERT INTO p(a) VALUES ('x')`} {
		if _, err := d.Exec(ctx, sql, nil); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	rows, err := d.Query(ctx, `SELECT bm25(p) FROM p WHERE p MATCH 'x'`, nil)
	if err != nil {
		if strings.Contains(err.Error(), "no such module") {
			t.Fatalf("the pure-Go SQLite build has no FTS5: %v", err)
		}
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		t.Fatal("bm25 probe returned no row")
	}
}

// Catches: a pure-Go build that lacks a tokenizer the declaration allows, or
// that cannot keep an external-content table in step (rebuild, delete command,
// integrity-check), reported as a raw driver string.
func TestFullTextSQLitePureTokenizersAndExternalContent(t *testing.T) {
	_, d := openPure(t, Config{})
	ctx := context.Background()
	run := func(sql string) {
		t.Helper()
		if _, err := d.Exec(ctx, sql, nil); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	run(`CREATE VIRTUAL TABLE porter_t USING fts5(a, tokenize='porter unicode61')`)
	run(`CREATE VIRTUAL TABLE trigram_t USING fts5(a, tokenize='trigram')`)
	run(`INSERT INTO porter_t(a) VALUES ('running dogs')`)
	run(`INSERT INTO trigram_t(a) VALUES ('substring match')`)
	count := func(sql string) int {
		t.Helper()
		rows, err := d.Query(ctx, sql, nil)
		if err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		defer func() { _ = rows.Close() }()
		var n int
		if !rows.Next() || rows.Scan(&n) != nil {
			t.Fatalf("%s: no count", sql)
		}
		return n
	}
	if n := count(`SELECT count(*) FROM porter_t WHERE porter_t MATCH 'run'`); n != 1 {
		t.Fatalf("porter stemming matched %d rows, want 1", n)
	}
	if n := count(`SELECT count(*) FROM trigram_t WHERE trigram_t MATCH 'bstr'`); n != 1 {
		t.Fatalf("trigram matched %d rows, want 1", n)
	}

	run(`CREATE TABLE art (id INTEGER PRIMARY KEY, title TEXT, body TEXT)`)
	run(`INSERT INTO art (id, title, body) VALUES (1, 'red fox', 'quick'), (2, 'blue whale', 'slow')`)
	run(`CREATE VIRTUAL TABLE art_fts USING fts5("title", "body", content='art', content_rowid='id', tokenize='porter unicode61')`)
	run(`INSERT INTO art_fts(art_fts) VALUES('rebuild')`)
	if n := count(`SELECT count(*) FROM art_fts WHERE art_fts MATCH 'fox'`); n != 1 {
		t.Fatalf("after rebuild: %d rows, want 1", n)
	}
	run(`INSERT INTO art_fts(art_fts, rowid, "title", "body") VALUES ('delete', 2, 'blue whale', 'slow')`)
	run(`DELETE FROM art WHERE id = 2`)
	run(`INSERT INTO art_fts(art_fts) VALUES('integrity-check')`)
}
