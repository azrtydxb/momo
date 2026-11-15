// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

func splitArticles(ft *schema.FullText, concurrently bool) *schema.Schema {
	t := schema.Table{
		Name: "articles",
		Columns: []schema.Column{
			{Name: "id", Type: "bigint", Identity: "by_default"},
			{Name: "title", Type: "text", Nullable: true},
			{Name: "body", Type: "text", Nullable: true},
		},
		PrimaryKey: []string{"id"},
	}
	if ft != nil {
		t.Indexes = []schema.Index{{Name: "articles_search", Method: "fulltext", Concurrently: concurrently,
			Parts: []schema.IndexPart{{Column: "title"}, {Column: "body"}}, FullText: ft}}
	}
	return &schema.Schema{Dialect: "postgres", Tables: []schema.Table{t}}
}

// splitDiff writes the migration for want on top of dir and adds its files, the
// no-transaction ones too, to dir.
func splitDiff(t *testing.T, dir fstest.MapFS, want *schema.Schema, name string, at time.Time) *migrate.Migration {
	t.Helper()
	e := migratetest.NewEngine(t, mpg.New(), nil, dir, migrate.WithClock(func() time.Time { return at }))
	m, err := e.Diff(context.Background(), want, name, migrate.DiffOptions{AllowDestructive: true})
	if err != nil || m == nil {
		t.Fatalf("diff %s: %v %v", name, m, err)
	}
	for n, b := range m.Files {
		dir[n] = &fstest.MapFile{Data: b}
	}
	return m
}

func sqlOf(stmts []migrate.Statement) string {
	var out []string
	for _, s := range stmts {
		out = append(out, s.SQL)
	}
	return strings.Join(out, ";\n")
}

func noBlockingIndex(t *testing.T, m *migrate.Migration) {
	t.Helper()
	parents := map[string]bool{"articles": true}
	for _, mm := range []*migrate.Migration{m, m.Next} {
		if mm == nil {
			continue
		}
		for _, f := range migrate.LintSQL(mm.Version, mm.Up, parents, "postgres") {
			if f.Rule == migrate.RuleBlockingIndex {
				t.Errorf("migration %s: lint reports %s: %s", mm.Version, f.Rule, f.Message)
			}
		}
	}
}

// Catches: a full-text declaration added to an existing table producing a plain
// CREATE INDEX that migrate lint rejects as blocking-index, or a concurrent
// build left in the transactional migration, or the column add moved out of it.
func TestFullTextConcurrentlyOnExistingTable(t *testing.T) {
	dir := fstest.MapFS{}
	splitDiff(t, dir, splitArticles(nil, false), "init", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ft := &schema.FullText{Columns: []string{"title", "body"}, Config: "english"}
	m := splitDiff(t, dir, splitArticles(ft, true), "search", time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))

	if m.NoTx || m.Next == nil || !m.Next.NoTx {
		t.Fatalf("want a transactional migration followed by a no-transaction one: %+v", m)
	}
	head, tail := sqlOf(m.Up), sqlOf(m.Next.Up)
	if !strings.Contains(head, `ADD COLUMN "articles_search_tsv" tsvector GENERATED ALWAYS AS (`) || !strings.Contains(head, `) STORED`) {
		t.Errorf("head lacks the generated column:\n%s", head)
	}
	if strings.Contains(head, "INDEX") {
		t.Errorf("head must not build the index:\n%s", head)
	}
	if tail != `CREATE INDEX CONCURRENTLY "articles_search" ON "articles" USING gin ("articles_search_tsv")` {
		t.Errorf("tail:\n%s", tail)
	}
	var tailFile string
	for n, b := range m.Files {
		if strings.Contains(string(b), "CREATE INDEX CONCURRENTLY") {
			tailFile = string(b)
			if !strings.HasSuffix(n, ".up.sql") {
				t.Errorf("concurrent index in %s", n)
			}
		}
	}
	if !strings.Contains(tailFile, "-- momo:no-transaction") {
		t.Errorf("the concurrent index file lacks -- momo:no-transaction:\n%s", tailFile)
	}
	noBlockingIndex(t, m)
}

// Catches: a config and weights change of a concurrent declaration being refused
// when the splitter renders the column change without the index change, the old
// index being dropped outside the transaction that recreates its column, or the
// rebuild being left in the head.
func TestFullTextConcurrentlyConfigChangeSplit(t *testing.T) {
	dir := fstest.MapFS{}
	splitDiff(t, dir, splitArticles(&schema.FullText{Columns: []string{"title", "body"}, Config: "english"}, true), "init", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	m := splitDiff(t, dir, splitArticles(&schema.FullText{Columns: []string{"title", "body"}, Config: "simple", Weights: []string{"A", "B"}}, true), "retune", time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))

	if m.NoTx || m.Next == nil || !m.Next.NoTx {
		t.Fatalf("want a transactional migration followed by a no-transaction one: %+v", m)
	}
	head, tail := sqlOf(m.Up), sqlOf(m.Next.Up)
	wantHead := strings.Join([]string{
		`DROP INDEX "articles_search"`,
		`ALTER TABLE "articles" DROP COLUMN "articles_search_tsv"`,
		`ALTER TABLE "articles" ADD COLUMN "articles_search_tsv" tsvector GENERATED ALWAYS AS (setweight(to_tsvector('simple', coalesce("title", '')), 'A') || setweight(to_tsvector('simple', coalesce("body", '')), 'B')) STORED`,
	}, ";\n")
	if head != wantHead {
		t.Errorf("head:\n%s\nwant:\n%s", head, wantHead)
	}
	if tail != `CREATE INDEX CONCURRENTLY "articles_search" ON "articles" USING gin ("articles_search_tsv")` {
		t.Errorf("tail:\n%s", tail)
	}
	noBlockingIndex(t, m)
}

// Catches: the change leaking into declarations that are not concurrent, which
// keep one transactional migration with a plain index (and so keep the
// blocking-index finding that asks for Concurrently).
func TestFullTextWithoutConcurrentlyUnchanged(t *testing.T) {
	dir := fstest.MapFS{}
	splitDiff(t, dir, splitArticles(nil, false), "init", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	m := splitDiff(t, dir, splitArticles(&schema.FullText{Columns: []string{"title", "body"}, Config: "english"}, false), "search", time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))
	if m.NoTx || m.Next != nil {
		t.Fatalf("a plain declaration is one transactional migration: %+v", m)
	}
	if !strings.Contains(sqlOf(m.Up), `CREATE INDEX "articles_search" ON "articles" USING gin ("articles_search_tsv")`) {
		t.Errorf("statements:\n%s", sqlOf(m.Up))
	}

	m2 := splitDiff(t, dir, splitArticles(&schema.FullText{Columns: []string{"title", "body"}, Config: "simple"}, false), "retune", time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	want := strings.Join([]string{
		`DROP INDEX "articles_search"`,
		`ALTER TABLE "articles" DROP COLUMN "articles_search_tsv"`,
		`ALTER TABLE "articles" ADD COLUMN "articles_search_tsv" tsvector GENERATED ALWAYS AS (to_tsvector('simple', coalesce("title", '')) || to_tsvector('simple', coalesce("body", ''))) STORED`,
		`CREATE INDEX "articles_search" ON "articles" USING gin ("articles_search_tsv")`,
	}, ";\n")
	if m2.Next != nil || sqlOf(m2.Up) != want {
		t.Errorf("retune:\n%s\nwant:\n%s", sqlOf(m2.Up), want)
	}
}
