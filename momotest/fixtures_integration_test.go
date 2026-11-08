// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momotest_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/momotest"
)

// fixtureRecTB records Fatalf and stops the calling goroutine, as the real one
// does, so a failing LoadFixtures can be observed without failing the test.
type fixtureRecTB struct {
	testing.TB
	mu   sync.Mutex
	errs []string
}

func (r *fixtureRecTB) Helper() {}

func (r *fixtureRecTB) Fatalf(format string, args ...any) {
	r.mu.Lock()
	r.errs = append(r.errs, fmt.Sprintf(format, args...))
	r.mu.Unlock()
	runtime.Goexit()
}

// loadRecorded runs LoadFixtures against a recording TB and returns what it
// reported through Fatalf.
func loadRecorded(t *testing.T, q momo.Querier, path string) []string {
	t.Helper()
	rt := &fixtureRecTB{TB: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		momotest.LoadFixtures(rt, q, path)
	}()
	<-done
	return rt.errs
}

// TestMomotestFixtures loads the YAML file into the shared relation fixture
// schema on every dialect testopen.Each provides (SQLite always, PostgreSQL
// when its URL is set).
func TestMomotestFixtures(t *testing.T) {
	rec := &obstest.Recorder{}
	testopen.Each(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		relfixture.CreateSchema(t, db)
		before := len(rec.SQLs())

		momotest.LoadFixtures(t, db, "testdata/fixtures.yaml")

		var ins []string
		for _, s := range rec.SQLs()[before:] {
			if strings.HasPrefix(s, "INSERT INTO") {
				ins = append(ins, s)
			}
		}
		if len(ins) != 4 {
			t.Fatalf("want 4 inserts, got %d: %q", len(ins), ins)
		}
		for i, table := range []string{`"tags"`, `"authors"`, `"documents"`, `"documents"`} {
			if !strings.Contains(ins[i], "INSERT INTO "+table) {
				t.Fatalf("insert %d = %q, want table %s (file order)", i, ins[i], table)
			}
		}
		if !strings.Contains(ins[2], `("author_id", "id", "title")`) {
			t.Fatalf("columns are not sorted and quoted: %q", ins[2])
		}
		docs, err := momo.Select[models.Document, *models.Document](db).OrderBy(document.ID.Asc()).All(t.Context())
		if err != nil || len(docs) != 2 {
			t.Fatalf("documents = %d err %v", len(docs), err)
		}
		if docs[0].AuthorID == nil || *docs[0].AuthorID != 1 || docs[1].AuthorID != nil {
			t.Fatalf("author ids = %v, %v; want 1 and NULL", docs[0].AuthorID, docs[1].AuthorID)
		}
		if docs[0].Title != "A1" || docs[1].Title != "Orphan" {
			t.Fatalf("titles = %q, %q", docs[0].Title, docs[1].Title)
		}

		errs := loadRecorded(t, db, "testdata/bad.yaml")
		if len(errs) != 1 || !strings.Contains(errs[0], "top level") || !strings.Contains(errs[0], "bad.yaml") {
			t.Fatalf("bad file errors = %v, want one naming the file and the top level", errs)
		}
		errs = loadRecorded(t, db, "testdata/missing.yaml")
		if len(errs) != 1 || !strings.Contains(errs[0], "missing.yaml") {
			t.Fatalf("missing file errors = %v, want one naming the file", errs)
		}
	}, momo.WithHook(rec))
}

// TestMomotestFixturesRowError: a row the database refuses fails the load with
// the table and the row, and the rows before it stay (each row is a statement).
func TestMomotestFixturesRowError(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		relfixture.CreateSchema(t, db)
		// documents row 1 references author 99, which does not exist.
		path := filepath.Join(t.TempDir(), "fk.yaml")
		doc := "authors:\n  - id: 1\n    name: Ada\ndocuments:\n  - id: 1\n    author_id: 99\n    title: Lost\n"
		if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
			t.Fatal(err)
		}
		errs := loadRecorded(t, db, path)
		if len(errs) != 1 || !strings.Contains(errs[0], "table documents row 1") {
			t.Fatalf("errors = %v, want one naming table documents row 1", errs)
		}
		n, err := momo.Scalars[int64](t.Context(), db, "SELECT count(*) FROM authors")
		if err != nil || len(n) != 1 || n[0] != 1 {
			t.Fatalf("authors after the failed load = %v, %v, want the row before the failure", n, err)
		}
	})
}
