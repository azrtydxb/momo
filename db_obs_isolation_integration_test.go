// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// TestNoHiddenGoroutines: queries, hooks and transactions start no goroutine of
// Momo's own.
func TestNoHiddenGoroutines(t *testing.T) {
	ctx := context.Background()
	testopen.Each(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		// Warm the pool so connection set-up goroutines of the driver exist
		// before the baseline; Momo itself must add none.
		if _, err := momo.Exec(ctx, db, `SELECT 1`); err != nil {
			t.Fatal(err)
		}
		runtime.GC()
		before := runtime.NumGoroutine()

		hdb := momo.New(db.Driver(), momo.WithHook(&obstest.Recorder{}))
		obsSeed(t, hdb)
		for i := 0; i < 200; i++ {
			obsDocs(t, hdb)
		}
		if err := hdb.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			_, err := momo.Exec(ctx, tx, `INSERT INTO authors (id, name) VALUES (?, ?)`, int64(70), "G")
			return err
		}); err != nil {
			t.Fatal(err)
		}

		deadline := time.Now().Add(2 * time.Second)
		for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		if after := runtime.NumGoroutine(); after > before {
			t.Fatalf("goroutines before=%d after=%d: Momo must start none", before, after)
		}
	})
}
