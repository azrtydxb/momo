// SPDX-License-Identifier: Apache-2.0

//go:build integration

package migrate_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
)

func pinned(t *testing.T, db *momo.DB) momo.PinnedConn {
	t.Helper()
	pc, err := db.Driver().(momo.Pinner).Pin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pc.Release() })
	return pc
}

// withPinned runs fn on a pinned connection and releases it when fn returns.
// Use it for one-shot work: pinned holds its connection until t.Cleanup, so a
// helper that called pinned on every use exhausted the pool (CI's holds four)
// and blocked the code under test in Acquire.
func withPinned(t *testing.T, db *momo.DB, fn func(c momo.PinnedConn)) {
	t.Helper()
	pc, err := db.Driver().(momo.Pinner).Pin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pc.Release() }()
	fn(pc)
}

func tableExists(t *testing.T, db *momo.DB, name string) bool {
	t.Helper()
	return firstValue(t, db, `SELECT (to_regclass('`+name+`') IS NOT NULL)::text`) == "true"
}
