// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
)

// KeysetPaginationMixedDirections walks a ten-row table in pages of three,
// ordered by version descending then tenant ascending, with After carrying the
// last row of each page as the cursor. Every row must be seen exactly once.
func KeysetPaginationMixedDirections(t *testing.T, db *momo.DB, caps Caps) {
	t.Helper()
	CreateDocuments(t, db, caps)
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title, version) VALUES (?, ?, ?)", int64(i%3), "doc", int64(i%2+1)); err != nil {
			t.Fatalf("seed row %d: %v", i, err)
		}
	}
	seen := map[int64]bool{}
	pages := 0
	q := momo.Select[Document](db).OrderBy(DocVersion.Desc(), DocTenantID.Asc()).Limit(3)
	page := q
	for {
		rows, err := page.All(ctx)
		if err != nil {
			t.Fatalf("page %d: %v", pages+1, err)
		}
		if len(rows) == 0 {
			break
		}
		pages++
		for _, r := range rows {
			if seen[r.ID] {
				t.Fatalf("row %d returned twice (page %d)", r.ID, pages)
			}
			seen[r.ID] = true
		}
		last := rows[len(rows)-1]
		page = q.After(last.Version, last.TenantID, last.ID)
		if pages > 10 {
			t.Fatal("keyset walk did not terminate")
		}
	}
	if len(seen) != 10 || pages < 4 {
		c, _ := page.SQL()
		t.Fatalf("saw %d rows over %d pages, want 10 rows over at least 4 pages; last page SQL %s args %v", len(seen), pages, c.SQL, c.Args)
	}
}

// KeysetRejectsNullableKey asserts that After over a nullable ORDER BY key
// returns ErrUnsupported while a non-nullable key builds.
func KeysetRejectsNullableKey(t *testing.T, db *momo.DB, caps Caps) {
	t.Helper()
	_, err := momo.Select[Document](db).OrderBy(DocBody.Asc()).After("x", int64(1)).SQL()
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("After over a nullable key: err = %v, want ErrUnsupported", err)
	}
	if _, err := momo.Select[Document](db).OrderBy(DocTitle.Asc()).After("x", int64(1)).SQL(); err != nil {
		t.Fatalf("After over a non-nullable key: %v", err)
	}
}
