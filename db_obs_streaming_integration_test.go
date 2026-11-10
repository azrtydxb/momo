// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"runtime"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

const (
	// streamRows is the number of rows TestStreamingBoundedMemory iterates.
	streamRows = 1_000_000
	// streamSample is how often (in rows) the test reads the heap.
	streamSample = 100_000
)

func heapAlloc() uint64 {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.HeapAlloc
}

// TestStreamingBoundedMemory iterates one million rows with Each and asserts
// the heap stays flat: Each must not buffer the result set.
func TestStreamingBoundedMemory(t *testing.T) {
	ctx := context.Background()
	testopen.Each(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		relfixture.CreateSchema(t, db)
		if _, err := momo.Exec(ctx, db, `INSERT INTO images (id, owner_type, owner_id, url)
WITH RECURSIVE c(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM c WHERE n < 1000000)
SELECT n, 'document', n, 'https://example.test/img/' || n FROM c`); err != nil {
			t.Fatal(err)
		}

		base := heapAlloc()
		var maxHeap uint64
		seen := 0
		err := momo.Select[models.Image](db).Each(ctx, func(*models.Image) error {
			seen++
			if seen%streamSample == 0 {
				if h := heapAlloc(); h > maxHeap {
					maxHeap = h
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if seen != streamRows {
			t.Fatalf("iterated %d rows, want %d", seen, streamRows)
		}
		if growth := int64(maxHeap) - int64(base); growth > streamMaxGrowB {
			t.Fatalf("heap grew %d bytes while streaming %d rows, bound %d: Each is buffering", growth, streamRows, streamMaxGrowB)
		}
	})
}
