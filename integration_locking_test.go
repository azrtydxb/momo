// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// TestLockingNowaitSkipLocked runs a two-transaction check: SKIP LOCKED passes
// over a row another transaction holds, NOWAIT fails with ErrBusy.
func TestLockingNowaitSkipLocked(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.LockingNowaitSkipLocked(t, db, suite.CapsOf(db))
	})
}

func TestWithTableNameQuoted(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.WithTableNameQuoted(t, db, suite.CapsOf(db))
	})
}

func TestHintsByDialect(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.HintsByDialect(t, db, suite.CapsOf(db))
	})
}
