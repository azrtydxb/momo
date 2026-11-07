// SPDX-License-Identifier: Apache-2.0

//go:build integration

package migrate_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
)

// firstValue returns the first column of the first row as a string.
func firstValue(t *testing.T, db *momo.DB, q string) string {
	t.Helper()
	var s string
	withPinned(t, db, func(c momo.PinnedConn) {
		r, err := c.Query(context.Background(), q, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = r.Close() }()
		if !r.Next() {
			t.Fatal("no row")
		}
		if err := r.Scan(&s); err != nil {
			t.Fatal(err)
		}
	})
	return s
}
