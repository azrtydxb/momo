// SPDX-License-Identifier: Apache-2.0

package migrate_test

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

func rebaseSchema(extra ...schema.Column) *schema.Schema {
	cols := append([]schema.Column{{Name: "id", Type: "INTEGER"}}, extra...)
	return &schema.Schema{Dialect: "postgres", Tables: []schema.Table{{Name: "t", Columns: cols, PrimaryKey: []string{"id"}}}}
}

// withFiles returns base plus the generated files of a migration; a later
// momo.sum replaces the earlier one.
func withFiles(base fstest.MapFS, files map[string][]byte) fstest.MapFS {
	out := fstest.MapFS{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range files {
		out[k] = &fstest.MapFile{Data: v}
	}
	return out
}

func diffInto(t *testing.T, dir fstest.MapFS, desired *schema.Schema, name string, now time.Time) (fstest.MapFS, *migrate.Migration) {
	t.Helper()
	m, err := offlineEngine(t, dir, now).Diff(context.Background(), desired, name, migrate.DiffOptions{})
	if err != nil || m == nil {
		t.Fatalf("diff %s: %v %v", name, m, err)
	}
	return withFiles(dir, m.Files), m
}

func TestMigrateRebase(t *testing.T) {
	ctx := context.Background()
	day := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	root, _ := diffInto(t, fstest.MapFS{}, rebaseSchema(), "root", day)

	// The target branch added column b after the shared root; this branch added a.
	target, _ := diffInto(t, root, rebaseSchema(schema.Column{Name: "b", Type: "TEXT", Nullable: true}), "add_b", day.Add(time.Hour))
	branch, mine := diffInto(t, root, rebaseSchema(schema.Column{Name: "a", Type: "TEXT", Nullable: true}), "add_a", day.Add(2*time.Hour))

	// After the merge the model has both columns.
	desired := rebaseSchema(schema.Column{Name: "a", Type: "TEXT", Nullable: true}, schema.Column{Name: "b", Type: "TEXT", Nullable: true})
	e := offlineEngine(t, branch, day.Add(3*time.Hour))
	removed, mig, err := e.Rebase(ctx, target, desired, "add_a")
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 2 || !strings.HasPrefix(removed[0], mine.Version+"_add_a.") || !strings.HasPrefix(removed[1], mine.Version+"_add_a.") {
		t.Fatalf("rebase must drop exactly the branch's generated up and snapshot files, removed = %v", removed)
	}
	tsnap, err := migrate.LatestSnapshot(target)
	if err != nil {
		t.Fatal(err)
	}
	if mig == nil || mig.Parent != tsnap.Version {
		t.Fatalf("rebase must regenerate on the target head %s, got %+v", tsnap.Version, mig)
	}
	if mig.Version <= tsnap.Version {
		t.Fatalf("regenerated version %s must come after the target head %s", mig.Version, tsnap.Version)
	}

	// The target plus the regenerated files is one valid chain, and the model
	// round-trips with no further change.
	merged := withFiles(target, mig.Files)
	rep, err := migrate.ValidateOffline(merged)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Problems) != 0 {
		t.Fatalf("rebased directory is invalid: %v", rep.Problems)
	}
	again, err := offlineEngine(t, merged, day.Add(4*time.Hour)).Diff(ctx, desired, "again", migrate.DiffOptions{})
	if err != nil || again != nil {
		t.Fatalf("a rebased directory must have no change left: %+v %v", again, err)
	}
}

func TestMigrateRebaseRefusesHandWrittenMigration(t *testing.T) {
	ctx := context.Background()
	day := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	root, _ := diffInto(t, fstest.MapFS{}, rebaseSchema(), "root", day)
	branch := withFiles(root, nil)
	branch["20260301120000_by_hand.up.sql"] = &fstest.MapFile{Data: []byte("ALTER TABLE t ADD COLUMN a text;\n")}

	_, mig, err := offlineEngine(t, branch, day.Add(3*time.Hour)).Rebase(ctx, root, rebaseSchema(), "x")
	if err == nil || mig != nil {
		t.Fatalf("want a refusal, got %+v %v", mig, err)
	}
	if !strings.Contains(err.Error(), "20260301120000_by_hand.up.sql") {
		t.Fatalf("the error must name the hand-written file: %v", err)
	}
}

func TestMigrateRebaseNothingToDo(t *testing.T) {
	ctx := context.Background()
	day := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	root, _ := diffInto(t, fstest.MapFS{}, rebaseSchema(), "root", day)
	removed, mig, err := offlineEngine(t, root, day.Add(time.Hour)).Rebase(ctx, root, rebaseSchema(), "x")
	if err != nil || mig != nil || len(removed) != 0 {
		t.Fatalf("identical branch and target: removed=%v mig=%+v err=%v", removed, mig, err)
	}
}
