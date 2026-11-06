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
			DefaultInValues: true, Merge: true, PartialIndexes: true, WindowFunctions: true, JSON: true, Arrays: true, NonDefaultIsolation: true},
		dialect.SQLite: {Dialect: dialect.SQLite, Returning: true, PartialIndexes: true, WindowFunctions: true},
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
	if m != suite.DocumentMeta || m.Table != "documents" {
		t.Fatalf("meta = %+v", m)
	}
	names := []string{"id", "tenant_id", "title", "body", "version", "deleted_at", "created_at", "updated_at", "secret"}
	if len(m.Columns) != len(names) {
		t.Fatalf("columns = %d, want %d", len(m.Columns), len(names))
	}
	for i, n := range names {
		if m.Columns[i].Name != n {
			t.Fatalf("Columns[%d] = %q, want %q", i, m.Columns[i].Name, n)
		}
	}
	if m.Identity != 0 || m.Version != 4 || m.SoftDelete != 5 || m.CreatedAt != 6 || m.UpdatedAt != 7 || len(m.PK) != 1 || m.PK[0] != 0 {
		t.Fatalf("derived indexes wrong: %+v", m)
	}
	if m.Columns[8].Flags != suite.DocSecret.Flags() || m.Columns[4].Flags != suite.DocVersion.Flags() {
		t.Fatal("column flags must equal the descriptors' flags")
	}
}

func TestDocumentMetaPtrTargetsFields(t *testing.T) {
	m := suite.DocumentMeta
	var d suite.Document
	*m.Ptr(&d, 2).(*string) = "hello"
	*m.Ptr(&d, 4).(*int64) = 7
	*m.Ptr(&d, 8).(*string) = "shh"
	if d.Title != "hello" || d.Version != 7 || d.Secret != "shh" {
		t.Fatalf("Ptr wrote to the wrong fields: %+v", d)
	}
	if m.Value(&d, 2) != "hello" || m.Value(&d, 4) != int64(7) || m.Value(&d, 8) != "shh" {
		t.Fatal("Value does not read back what Ptr wrote")
	}
}

func TestDocumentMetaTableSchema(t *testing.T) {
	m := suite.DocumentMeta
	if m.TableSchema == nil {
		t.Fatal("TableSchema is nil: the meta would read as a projection")
	}
	if m.TableSchema.Name != m.Table || len(m.TableSchema.Columns) != len(m.Columns) {
		t.Fatalf("table schema = %+v", m.TableSchema)
	}
	for i, c := range m.TableSchema.Columns {
		if c.Name != m.Columns[i].Name {
			t.Fatalf("TableSchema.Columns[%d] = %q, Columns[%d] = %q", i, c.Name, i, m.Columns[i].Name)
		}
	}
}
