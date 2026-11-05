// SPDX-License-Identifier: Apache-2.0

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

// TestCapabilitiesOfRows pins one capability row per dialect. SQLite and MySQL
// have no driver yet, so their rows are checked from the dialect description
// alone; they are never skipped silently.
func TestCapabilitiesOfRows(t *testing.T) {
	want := map[dialect.Name]suite.Caps{
		dialect.Postgres: {Dialect: dialect.Postgres, Returning: true, Locking: true, DefaultInUpdate: true,
			DefaultInValues: true, Merge: true, WindowFunctions: true, JSON: true, Arrays: true, NonDefaultIsolation: true},
		dialect.SQLite: {Dialect: dialect.SQLite, Returning: true, DefaultInValues: true, WindowFunctions: true},
		dialect.MySQL: {Dialect: dialect.MySQL, Locking: true, DefaultInUpdate: true, DefaultInValues: true,
			WindowFunctions: true, NonDefaultIsolation: true},
	}
	for name, w := range want {
		t.Run(string(name), func(t *testing.T) {
			if got := suite.CapabilitiesOf(dialect.For(name)); got != w {
				t.Fatalf("CapabilitiesOf(%s)\n got %+v\nwant %+v", name, got, w)
			}
		})
	}
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

func TestDocumentMeta(t *testing.T) {
	m := (*suite.Document)(nil).MomoMeta()
	if m != suite.DocumentMeta || m.Table != "documents" || len(m.Columns) != 9 {
		t.Fatalf("meta = %+v", m)
	}
	if m.Identity != 0 || m.Version != 4 || m.SoftDelete != 5 || m.CreatedAt != 6 || m.UpdatedAt != 7 || len(m.PK) != 1 || m.PK[0] != 0 {
		t.Fatalf("derived indexes wrong: %+v", m)
	}
	body := "x"
	d := suite.Document{Body: &body, Secret: "s"}
	for i, c := range m.Columns {
		if m.Ptr(&d, i) == nil {
			t.Fatalf("Ptr(%d) nil for %s", i, c.Name)
		}
		if m.Value(&d, i) == nil && c.Name != "deleted_at" {
			t.Fatalf("Value(%d) nil for %s", i, c.Name)
		}
	}
	if m.Columns[8].Flags != suite.DocSecret.Flags() || m.Columns[4].Flags != suite.DocVersion.Flags() {
		t.Fatal("column flags must equal the descriptors' flags")
	}
}
