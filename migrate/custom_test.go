// SPDX-License-Identifier: Apache-2.0

package migrate_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
)

// initDir is a directory holding the generated init migration of docsSchema.
func initDir(t *testing.T) fstest.MapFS {
	t.Helper()
	e := offlineEngine(t, fstest.MapFS{}, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	mig, err := e.Diff(context.Background(), docsSchema(), "init", migrate.DiffOptions{})
	if err != nil || mig == nil {
		t.Fatalf("diff: %v %v", mig, err)
	}
	return mapFS(mig.Files)
}

func TestNewCustomWritesChainedFiles(t *testing.T) {
	dir := initDir(t)
	e := offlineEngine(t, dir, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))
	m, err := e.NewCustom("backfill_slug")
	if err != nil {
		t.Fatal(err)
	}
	if m.Version != "20260201000000" || m.Parent != "20260102030405" {
		t.Fatalf("version/parent = %s/%s", m.Version, m.Parent)
	}
	up := string(m.Files["20260201000000_backfill_slug.up.sql"])
	if strings.Contains(up, "momo:generated") {
		t.Fatalf("a custom migration must not carry the generated directive:\n%s", up)
	}
	if d, err := migrate.ParseDirectives([]byte(up)); err != nil || d.Generated {
		t.Fatalf("directives %+v, %v", d, err)
	}
	snap, err := migrate.ReadSnapshot(bytes.NewReader(m.Files["20260201000000_backfill_slug.snapshot.json"]))
	if err != nil || snap.ParentVersion != "20260102030405" || snap.Version != "20260201000000" {
		t.Fatalf("snapshot %+v, %v", snap, err)
	}
	// The result is a valid directory: the sum covers the new files, the chain
	// links, and the snapshot equals its parent's.
	merged := fstest.MapFS{}
	for k, v := range dir {
		merged[k] = v
	}
	for k, v := range m.Files {
		merged[k] = &fstest.MapFile{Data: v}
	}
	rep, err := migrate.ValidateOffline(merged)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Problems) != 0 {
		t.Fatalf("validate: %v", rep.Problems)
	}
	sum, err := migrate.ParseSum(bytes.NewReader(m.Files["momo.sum"]))
	if err != nil || len(sum.Files) != 4 {
		t.Fatalf("momo.sum = %+v, %v", sum, err)
	}
}

func TestNewCustomRefusals(t *testing.T) {
	e := offlineEngine(t, fstest.MapFS{}, time.Now())
	if _, err := e.NewCustom("x"); !errors.Is(err, migrate.ErrNoSnapshot) {
		t.Fatalf("empty directory: want ErrNoSnapshot, got %v", err)
	}
	e = offlineEngine(t, initDir(t), time.Now())
	for _, name := range []string{"Bad Name", "", "1abc", "a-b"} {
		if _, err := e.NewCustom(name); err == nil || !strings.Contains(err.Error(), "must match") {
			t.Fatalf("name %q: want a name error, got %v", name, err)
		}
	}
}

// With vector external, Diff emits no CREATE EXTENSION and the snapshot holds no
// extension; without it the same models emit it.
func TestDiffExternalExtensionsNeverEmitted(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	ext := migratetest.NewEngine(t, mpg.New(), nil, fstest.MapFS{}, migrate.WithClock(func() time.Time { return now }),
		migrate.WithConfig(migrate.Config{ExternalExtensions: []string{"vector"}}))
	m, err := ext.Diff(ctx, docsSchema(), "init", migrate.DiffOptions{})
	if err != nil || m == nil {
		t.Fatalf("diff: %v %v", m, err)
	}
	if up := string(m.Files["20260102030405_init.up.sql"]); strings.Contains(up, "EXTENSION") {
		t.Fatalf("external extension emitted:\n%s", up)
	}
	snap, err := migrate.ReadSnapshot(bytes.NewReader(m.Files["20260102030405_init.snapshot.json"]))
	if err != nil || len(snap.Schema.Extensions) != 0 {
		t.Fatalf("snapshot extensions = %+v, %v", snap.Schema.Extensions, err)
	}
	// A second diff against the same models finds nothing to do.
	again := migratetest.NewEngine(t, mpg.New(), nil, mapFS(m.Files), migrate.WithClock(func() time.Time { return now.Add(time.Hour) }),
		migrate.WithConfig(migrate.Config{ExternalExtensions: []string{"vector"}}))
	if m2, err := again.Diff(ctx, docsSchema(), "again", migrate.DiffOptions{}); err != nil || m2 != nil {
		t.Fatalf("second diff = %v, %v", m2, err)
	}

	plain := offlineEngine(t, fstest.MapFS{}, now)
	m, err = plain.Diff(ctx, docsSchema(), "init", migrate.DiffOptions{})
	if err != nil || m == nil || !strings.Contains(string(m.Files["20260102030405_init.up.sql"]), "CREATE EXTENSION IF NOT EXISTS") {
		t.Fatalf("control: the extension must be emitted without the setting: %v", err)
	}
}
