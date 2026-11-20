// SPDX-License-Identifier: Apache-2.0

package migrate_test

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// Catches: an exclusion constraint that replaces a concurrently dropped index
// of the same name staying in the transactional head. The index is dropped in
// the no-transaction tail, so the head would add the constraint (and its
// backing index of that name) while the old index still exists, and the
// migration would fail with "relation already exists".
func TestDiffExcludeFollowsConcurrentIndexDropOfTheSameName(t *testing.T) {
	ctx := context.Background()
	from := docsSchema()
	from.Extensions = append(from.Extensions, schema.Extension{Name: "btree_gist"})
	from.Tables[0].Indexes = []schema.Index{{Name: "docs_title", Concurrently: true, Parts: []schema.IndexPart{{Column: "title"}}}}
	first := offlineEngine(t, fstest.MapFS{}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	m1, err := first.Diff(ctx, from, "init", migrate.DiffOptions{})
	if err != nil || m1 == nil {
		t.Fatal(m1, err)
	}

	to := docsSchema()
	to.Extensions = append(to.Extensions, schema.Extension{Name: "btree_gist"})
	to.Tables[0].Indexes = nil
	// A transactional change keeps a head to split from: with only the drop
	// and the constraint, nothing is left for the head and the whole plan is
	// one no-transaction migration with no Next.
	to.Tables[0].Columns = append(to.Tables[0].Columns, schema.Column{Name: "note", Type: "TEXT", Nullable: true})
	to.Tables[0].Excludes = []schema.Exclude{{Name: "docs_title", Elements: []schema.ExcludeElement{{Column: "title", Operator: "="}}}}
	m, err := offlineEngine(t, mapFS(m1.Files), lockfreeClock).Diff(ctx, to, "swap", migrate.DiffOptions{})
	if err != nil || m == nil {
		t.Fatal(m, err)
	}
	var all, head string
	for name, b := range m.Files {
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		all += string(b)
		if !strings.Contains(string(b), "-- momo:no-transaction") {
			head += string(b)
		}
	}
	if strings.Contains(head, "EXCLUDE") {
		t.Fatalf("the exclusion constraint stayed in the transactional head:\n%s", head)
	}
	drop, add := strings.Index(all, `DROP INDEX CONCURRENTLY`), strings.Index(all, "EXCLUDE USING gist")
	if drop < 0 || add < 0 || drop > add {
		t.Fatalf("want the concurrent drop before the exclusion constraint, in the tail:\n%s", all)
	}
	if m.Next == nil || !m.Next.NoTx {
		t.Fatalf("want a no-transaction tail, got %+v", m.Next)
	}
	// The head's snapshot does not have the constraint the tail adds.
	if h := m.Snapshot; h != nil {
		for _, tb := range h.Tables {
			if len(tb.Excludes) != 0 {
				t.Errorf("the head snapshot already holds the exclusion constraint: %+v", tb.Excludes)
			}
		}
	}
}
