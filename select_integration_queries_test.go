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

func TestIntegrationQueries(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) { suite.Queries(t, db, suite.CapsOf(db)) })
}

func TestSuiteRunAll(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) { suite.RunAll(t, db, suite.CapsOf(db)) })
}
