// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"sync"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// Two Replace calls on the same parent race on separate pooled connections.
// Starting from {1,2}, A wants {1,3,4} and B wants {2,3,5}: without the parent
// row lock both read {1,2} and then both insert tag 3, so one fails with a
// unique violation (or the final set mixes the inputs). With the lock they run
// one after the other and the result is exactly one of the two inputs.
func TestAssociationConcurrentReplaceSameParent(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if !db.Dialect().Locking {
			t.Skip("Replace takes a parent row lock only on dialects with row locks")
		}
		relfixture.CreateSchema(t, db)
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		doc := &models.Document{ID: 2}
		a, b := tagList(1, 1), tagList(2, 2)
		a = append(a, tagList(3, 4)...)
		b = append(b, tagList(3, 3)...)
		b = append(b, tagList(5, 5)...)
		for i := 0; i < 20; i++ {
			mustExec(t, db, `DELETE FROM taggings WHERE document_id = 2`)
			mustExec(t, db, `INSERT INTO taggings (document_id, tag_id) VALUES (2, 1), (2, 2)`)
			start := make(chan struct{})
			errs := make([]error, 2)
			var wg sync.WaitGroup
			for n, set := range [][]*models.Tag{a, b} {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					errs[n] = document.Tags.Replace(ctx, db, doc, set...)
				}()
			}
			close(start)
			wg.Wait()
			for n, err := range errs {
				if err != nil {
					t.Fatalf("iteration %d: Replace %c: %v", i, 'A'+n, err)
				}
			}
			got := assocQueryIDs(t, db, `SELECT tag_id FROM taggings WHERE document_id = 2 ORDER BY tag_id`)
			if !sameIDs(got, 1, 3, 4) && !sameIDs(got, 2, 3, 5) {
				t.Fatalf("iteration %d: final set %v is neither {1,3,4} nor {2,3,5}", i, got)
			}
		}
	})
}
