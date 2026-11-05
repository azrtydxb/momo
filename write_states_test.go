// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestWriteStates(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) { suite.WriteStates(t, db, suite.CapsOf(db)) })
}

func TestWriteStatesInsert(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) { suite.WriteStatesInsert(t, db, suite.CapsOf(db)) })
}

func TestWriteStatesMultiRow(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) { suite.WriteStatesMultiRow(t, db, suite.CapsOf(db)) })
}
