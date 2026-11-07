// SPDX-License-Identifier: Apache-2.0

//go:build integration

package suite_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestHarnessCreatesDocuments(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		if _, err := momo.Exec(context.Background(), db, "INSERT INTO documents (tenant_id, title) VALUES (1, 'probe')"); err != nil {
			t.Fatal(err)
		}
	})
}

func TestCapsOfAddsDriverCapabilities(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		c := suite.CapsOf(db)
		if c.Dialect != d {
			t.Fatalf("Dialect = %s, want %s", c.Dialect, d)
		}
		rep, ok := db.Driver().(momo.CapabilityReporter)
		if want := ok && rep.Capabilities().Vectors; want != c.Vectors {
			t.Fatalf("Vectors = %v, driver reports %v", c.Vectors, want)
		}
		if _, ok := db.Driver().(momo.Copier); ok != c.Copy {
			t.Fatalf("Copy = %v, driver Copier = %v", c.Copy, ok)
		}
		if _, ok := db.Driver().(momo.Batcher); ok != c.Batch {
			t.Fatalf("Batch = %v, driver Batcher = %v", c.Batch, ok)
		}
	})
}
