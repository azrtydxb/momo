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

func TestMergeAndBulkUpdate(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) { suite.MergeAndBulkUpdate(t, db, suite.CapsOf(db)) })
}
