// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"context"
	"testing"

	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	"github.com/azrtydxb/momo/schema"
)

// tsvectorSchema is the NovaCRM shape: a knowledge-base table whose tsvector
// column is kept current by a trigger function and searched through a GIN
// index.
func tsvectorSchema() *schema.Schema {
	return &schema.Schema{
		Dialect: "postgres",
		Tables: []schema.Table{{
			Name: "kb_articles",
			Columns: []schema.Column{
				{Name: "id", Type: "bigint", Identity: "by_default"},
				{Name: "title", Type: "text"},
				{Name: "body", Type: "text"},
				{Name: "search_tsv", Type: "tsvector", Nullable: true},
			},
			PrimaryKey: []string{"id"},
			Indexes: []schema.Index{{
				Name: "kb_articles_search_gin", Method: "gin",
				Parts: []schema.IndexPart{{Column: "search_tsv"}},
			}},
		}},
		Functions: []schema.Function{{
			Name: "kb_articles_tsv", Returns: "trigger", Language: "plpgsql", Volatility: "VOLATILE",
			Body: "BEGIN NEW.search_tsv := setweight(to_tsvector('english', coalesce(NEW.title, '')), 'A') || " +
				"to_tsvector('english', coalesce(NEW.body, '')); RETURN NEW; END;",
		}},
		Triggers: []schema.Trigger{{
			Table: "kb_articles", Name: "kb_articles_tsv_trg", Timing: "BEFORE", Events: []string{"INSERT", "UPDATE"},
			ForEachRow: true, Function: "kb_articles_tsv()",
		}},
	}
}

// TestTSVectorMigrationRoundTrip applies a tsvector column, its GIN index and
// the maintaining trigger through the engine, proves the trigger fills the
// column, and reads everything back with no diff and an empty second diff.
func TestTSVectorMigrationRoundTrip(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	desired := tsvectorSchema()
	files := migrationFiles(planStatements(t, &schema.Schema{Dialect: "postgres"}, desired, migrate.DiffOptions{}), 0)
	if _, err := engineOn(t, db, migratetest.FS(files)).Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}

	mustExec(t, db.Driver(), `INSERT INTO kb_articles (title, body) VALUES ('Reset password', 'Click the link to reset it')`)
	rows, err := db.Driver().Query(ctx, `SELECT search_tsv @@ websearch_to_tsquery('english', 'resetting') FROM kb_articles`, nil)
	if err != nil {
		t.Fatal(err)
	}
	var matched bool
	if !rows.Next() {
		t.Fatal("row missing")
	}
	if err := rows.Scan(&matched); err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()
	if !matched {
		t.Fatal("the trigger did not fill the tsvector column")
	}

	got := inspectNow(t, db)
	desired.Extensions = got.Extensions // database-wide; testdb installs vector
	if !schema.Equal(got, desired) {
		t.Fatalf("read-back has a diff:\n%s", migratetest.Diff(got, desired))
	}
	if again, _, err := migrate.Diff(got, desired, migrate.DiffOptions{}); err != nil || len(again) != 0 {
		t.Fatalf("second diff must be empty: %v %+v", err, again)
	}
	tbl, _ := got.Table("kb_articles")
	if c, ok := tbl.Column("search_tsv"); !ok || c.Type != "tsvector" {
		t.Fatalf("search_tsv = %+v, want type tsvector", c)
	}
}
