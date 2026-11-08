// SPDX-License-Identifier: Apache-2.0

package migrate_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

// An existing snapshot that already declares vector, with vector now external,
// produces no migration and no DROP, whether the models still declare the
// extension or not.
func TestDiffExternalExtensionAlreadyInSnapshot(t *testing.T) {
	dir := initDir(t) // its snapshot holds the vector extension
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	e := migratetest.NewEngine(t, mpg.New(), nil, dir, migrate.WithClock(func() time.Time { return now }),
		migrate.WithConfig(migrate.Config{ExternalExtensions: []string{"vector"}}))

	withoutVector := docsSchema()
	withoutVector.Extensions = nil
	for name, desired := range map[string]*schema.Schema{
		"models still declare vector":     docsSchema(),
		"models no longer declare vector": withoutVector,
	} {
		m, err := e.Diff(context.Background(), desired, "next", migrate.DiffOptions{})
		if err != nil || m != nil {
			t.Fatalf("%s: want no migration, got %v, %v", name, m, err)
		}
	}

	// A change next to it still produces a migration, with no extension or DROP statement.
	changed := docsSchema()
	changed.Tables[0].Columns = append(changed.Tables[0].Columns, schema.Column{Name: "note", Type: "TEXT", Nullable: true})
	m, err := e.Diff(context.Background(), changed, "note", migrate.DiffOptions{})
	if err != nil || m == nil {
		t.Fatalf("diff = %v, %v", m, err)
	}
	for _, s := range m.Up {
		if strings.Contains(s.SQL, "EXTENSION") || strings.HasPrefix(s.SQL, "DROP") {
			t.Fatalf("unexpected statement %q", s.SQL)
		}
	}
	if len(m.Snapshot.Extensions) != 0 {
		t.Fatalf("the new snapshot must not hold the external extension: %+v", m.Snapshot.Extensions)
	}
}
