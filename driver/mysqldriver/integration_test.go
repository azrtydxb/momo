// SPDX-License-Identifier: Apache-2.0

//go:build integration

package mysqldriver_test

import (
	"testing"

	"github.com/azrtydxb/momo/internal/suite"
)

// TestMySQLIntegration runs the shared transaction, error, query and write
// suites on MySQL 8.4, MySQL 9.7 and MariaDB 11.8.
func TestMySQLIntegration(t *testing.T) {
	for _, s := range servers {
		t.Run(s.name, func(t *testing.T) {
			db := s.open(t)
			suite.RunAll(t, db, suite.CapsOf(db))
		})
	}
}
