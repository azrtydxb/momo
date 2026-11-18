// SPDX-License-Identifier: Apache-2.0

package migrate_test

import (
	"bytes"
	"context"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

// walkCountFS counts the directory reads made through it, one per directory a
// walk lists, so a test can tell how many times a function walked the tree.
type walkCountFS struct {
	fstest.MapFS
	reads *int
}

func newWalkCountFS(files fstest.MapFS) walkCountFS {
	return walkCountFS{MapFS: files, reads: new(int)}
}

func (w walkCountFS) ReadDir(name string) ([]fs.DirEntry, error) {
	*w.reads++
	return w.MapFS.ReadDir(name)
}

// withSumFile returns dir plus a momo.sum covering it, so an engine reads the
// sum instead of recomputing it from a second walk.
func withSumFile(t *testing.T, dir fstest.MapFS) fstest.MapFS {
	t.Helper()
	sum, err := migrate.ComputeSum(dir)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := sum.Write(&buf); err != nil {
		t.Fatal(err)
	}
	return withFiles(dir, map[string][]byte{migrate.SumFileName: buf.Bytes()})
}

// oneWalk is how many directory reads a single Discover of dir makes.
func oneWalk(t *testing.T, dir fstest.MapFS) int {
	t.Helper()
	c := newWalkCountFS(dir)
	if _, err := migrate.Discover(c); err != nil {
		t.Fatal(err)
	}
	if *c.reads == 0 {
		t.Fatal("Discover made no directory read; the counter is not wired")
	}
	return *c.reads
}

func TestLatestStateWalksOnce(t *testing.T) {
	dir := initDir(t)
	want := oneWalk(t, dir)
	c := newWalkCountFS(dir)
	if _, err := migrate.LatestState(c); err != nil {
		t.Fatal(err)
	}
	if *c.reads != want {
		t.Fatalf("LatestState made %d directory reads, want %d (one walk)", *c.reads, want)
	}
}

func TestNewCustomWalksOnce(t *testing.T) {
	dir := withSumFile(t, initDir(t))
	want := oneWalk(t, dir)
	c := newWalkCountFS(dir)
	e := migratetest.NewEngine(t, mpg.New(), nil, c, migrate.WithClock(func() time.Time { return time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC) }))
	if _, err := e.NewCustom("backfill"); err != nil {
		t.Fatal(err)
	}
	if *c.reads != want {
		t.Fatalf("NewCustom made %d directory reads, want %d (one walk)", *c.reads, want)
	}
}

// TestRebaseDoesNotRediscoverTarget compares Rebase with the work it must do
// anyway: ValidateOffline of the target (one directory check, one Discover,
// the sum check) and the Diff on the target. Rebase discovers the target once
// itself and hands the files to the validation, so its reads are those two
// minus the validation's own directory check.
func TestRebaseDoesNotRediscoverTarget(t *testing.T) {
	ctx := context.Background()
	day := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	root, _ := diffInto(t, fstest.MapFS{}, rebaseSchema(), "root", day)
	target, _ := diffInto(t, root, rebaseSchema(schema.Column{Name: "b", Type: "TEXT", Nullable: true}), "add_b", day.Add(time.Hour))
	target = withSumFile(t, target)
	branch, _ := diffInto(t, root, rebaseSchema(schema.Column{Name: "a", Type: "TEXT", Nullable: true}), "add_a", day.Add(2*time.Hour))
	desired := rebaseSchema(schema.Column{Name: "a", Type: "TEXT", Nullable: true}, schema.Column{Name: "b", Type: "TEXT", Nullable: true})
	now := day.Add(3 * time.Hour)

	vc := newWalkCountFS(target)
	if _, err := migrate.ValidateOffline(vc); err != nil {
		t.Fatal(err)
	}
	validateReads := *vc.reads

	dc := newWalkCountFS(target)
	de := migratetest.NewEngine(t, mpg.New(), nil, dc, migrate.WithClock(func() time.Time { return now }))
	if _, err := de.Diff(ctx, desired, "add_a", migrate.DiffOptions{}); err != nil {
		t.Fatal(err)
	}
	diffReads := *dc.reads

	rc := newWalkCountFS(target)
	if _, _, err := offlineEngine(t, branch, now).Rebase(ctx, rc, desired, "add_a"); err != nil {
		t.Fatal(err)
	}
	// ValidateOffline starts with one ReadDir of the root that Rebase's own
	// Discover already covers.
	if want := validateReads - 1 + diffReads; *rc.reads != want {
		t.Fatalf("Rebase made %d reads of the target, want %d (validate %d minus its root check, plus diff %d)", *rc.reads, want, validateReads, diffReads)
	}
}
