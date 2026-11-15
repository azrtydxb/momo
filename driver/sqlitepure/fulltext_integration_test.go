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
