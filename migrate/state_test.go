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
	"github.com/azrtydxb/momo/schema"
)

func tableNames(s *schema.Schema) []string {
	var out []string
	for _, t := range s.Tables {
		out = append(out, t.Name)
	}
	return out
}

func sameStrings(a, b []string) bool { return strings.Join(a, ",") == strings.Join(b, ",") }

// After two branches merged, the derived state holds the work of both; neither
// branch's own snapshot file does.
func TestLatestStateAfterMerge(t *testing.T) {
	merged := pbMergedIndependent(t)
	st, err := migrate.LatestState(merged)
	if err != nil {
		t.Fatal(err)
	}
	if st.Version != pbVB {
		t.Fatalf("latest version = %s, want %s", st.Version, pbVB)
	}
	if got := tableNames(st.Schema); !sameStrings(got, []string{"a", "b", "base"}) {
		t.Fatalf("derived state tables = %v, want a, b, base", got)
	}
	stored, err := migrate.LatestSnapshot(merged)
	if err != nil {
		t.Fatal(err)
	}
	if got := tableNames(stored.Schema); sameStrings(got, []string{"a", "b", "base"}) {
		t.Fatalf("the stored snapshot is one branch's view, got %v", got)
	}
}

// A linear chain derives to its stored snapshots.
func TestLatestStateLinearIsTheStoredSnapshot(t *testing.T) {
	m := fstest.MapFS{
		pbV0 + "_init.snapshot.json": pbFile(pbSnapshot(pbV0, "", pbTable("base"))),
		pbVA + "_a.snapshot.json":    pbFile(pbSnapshot(pbVA, pbV0, pbTable("a"))),
	}
	st, err := migrate.LatestState(m)
	if err != nil {
		t.Fatal(err)
	}
	if got := tableNames(st.Schema); !sameStrings(got, []string{"a"}) {
		t.Fatalf("a linear chain is its stored snapshot, got %v", got)
	}
	if _, err := migrate.LatestState(fstest.MapFS{}); !errors.Is(err, migrate.ErrNoSnapshot) {
		t.Fatalf("want ErrNoSnapshot, got %v", err)
	}
}

func TestLatestStateConflict(t *testing.T) {
	base := pbBase()
	a := pbBranch(base, pbVA, "add_x", "ALTER TABLE base ADD COLUMN x text;", pbTable("base", "x"))
	b := pbBranch(base, pbVB, "add_y", "ALTER TABLE base ADD COLUMN y text;", pbTable("base", "y"))
	if _, err := migrate.LatestState(pbMerge(t, a, b)); !errors.Is(err, migrate.ErrBranchConflict) {
		t.Fatalf("want ErrBranchConflict, got %v", err)
	}
}

// Both branches making the identical change to a table is a conflict: both
// SQL files would run, and the second would fail on apply.
func TestForkWithSameChangeConflicts(t *testing.T) {
	base := pbBase()
	a := pbBranch(base, pbVA, "add_a", "CREATE TABLE a (id bigint PRIMARY KEY);", pbTable("base"), pbTable("a"))
	b := pbBranch(base, pbVB, "add_a_too", "CREATE TABLE a (id bigint PRIMARY KEY);", pbTable("base"), pbTable("a"))
	_, err := migrate.LatestState(pbMerge(t, a, b))
	if !errors.Is(err, migrate.ErrBranchConflict) || !strings.Contains(err.Error(), `table "a"`) {
		t.Fatalf("identical changes on both branches must conflict, got %v", err)
	}
}

// A migration built on one branch's migration after the merge keeps the other
// branch's work: P, A1, B1 (from P), B2 (from B1).
func TestLatestStateChildOfMergedFork(t *testing.T) {
	merged := pbMergedIndependent(t)
	b2 := pbBranch(merged, "20260103000000", "add_b2", "CREATE TABLE b2 (id bigint PRIMARY KEY);", pbTable("base"), pbTable("b"), pbTable("b2"))
	b2["20260103000000_add_b2.snapshot.json"] = pbFile(pbSnapshot("20260103000000", pbVB, pbTable("base"), pbTable("b"), pbTable("b2")))
	b2 = pbMerge(t, b2, fstest.MapFS{})
	if p := pbProblems(t, b2); len(p) != 0 {
		t.Fatalf("validate: %v", p)
	}
	st, err := migrate.LatestState(b2)
	if err != nil {
		t.Fatal(err)
	}
	if got := tableNames(st.Schema); !sameStrings(got, []string{"a", "b", "b2", "base"}) {
		t.Fatalf("the child of a merged fork must keep the other branch's work, got %v", got)
	}
}

// Renumbering a branch of two migrations leaves a child of a merged fork.
func TestRenumberTwoMigrationBranch(t *testing.T) {
	base := pbBase()
	a1 := pbBranch(base, pbVA, "add_a1", "CREATE TABLE a1 (id bigint PRIMARY KEY);", pbTable("base"), pbTable("a1"))
	a1["20260102000100_add_a2.up.sql"] = pbFile("-- momo:generated\nCREATE TABLE a2 (id bigint PRIMARY KEY);\n")
	a1["20260102000100_add_a2.snapshot.json"] = pbFile(pbSnapshot("20260102000100", pbVA, pbTable("base"), pbTable("a1"), pbTable("a2")))
	b := pbBranch(base, pbVB, "add_b", "CREATE TABLE b (id bigint PRIMARY KEY);", pbTable("base"), pbTable("b"))
	dir := pbMerge(t, a1, b)
	e := offlineEngine(t, dir, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	removed, files, moved, err := e.Renumber([]string{pbV0, pbVB})
	if err != nil || len(moved) != 2 {
		t.Fatalf("renumber: %+v %v", moved, err)
	}
	next := pbApplyRenumber(dir, removed, files)
	if p := pbProblems(t, next); len(p) != 0 {
		t.Fatalf("validate: %v", p)
	}
	st, err := migrate.LatestState(next)
	if err != nil {
		t.Fatal(err)
	}
	if got := tableNames(st.Schema); !sameStrings(got, []string{"a1", "a2", "b", "base"}) {
		t.Fatalf("after renumber the state must hold both branches, got %v", got)
	}
}

// A diff after a merge builds on the derived state, and a later deliberate
// drop of the other branch's table is a change, not swallowed by the fold.
func TestDiffAfterMergeThenDeliberateDrop(t *testing.T) {
	ctx := context.Background()
	merged := pbMergedIndependent(t)
	m1, err := offlineEngine(t, merged, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)).Diff(ctx, pbDesired("base", "a", "b", "c"), "add_c", migrate.DiffOptions{})
	if err != nil || m1 == nil {
		t.Fatalf("diff: %v %v", m1, err)
	}
	next := withFiles(merged, m1.Files)
	m2, err := offlineEngine(t, next, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)).Diff(ctx, pbDesired("base", "b", "c"), "drop_a", migrate.DiffOptions{AllowDestructive: true})
	if err != nil || m2 == nil {
		t.Fatalf("drop diff: %v %v", m2, err)
	}
	last := withFiles(next, m2.Files)
	if p := pbProblems(t, last); len(p) != 0 {
		t.Fatalf("validate: %v", p)
	}
	st, err := migrate.LatestState(last)
	if err != nil || !sameStrings(tableNames(st.Schema), []string{"b", "base", "c"}) {
		t.Fatalf("the drop must hold: %v %v", st, err)
	}
}

// A branch that drops a table the other branch's table refers to is refused by
// the foreign-key check of the merged state.
func TestForkDroppingReferencedTableConflicts(t *testing.T) {
	root := fstest.MapFS{
		pbV0 + "_init.up.sql":        pbFile("-- momo:generated\nCREATE TABLE p (id bigint);\n"),
		pbV0 + "_init.snapshot.json": pbFile(pbSnapshot(pbV0, "", pbTable("p"))),
	}
	child := `{"name":"c","columns":[{"name":"id","type":"bigint"},{"name":"p_id","type":"bigint"}],"foreign_keys":[{"name":"c_p","columns":["p_id"],"ref_table":"p","ref_columns":["id"]}]}`
	drop := pbBranch(root, pbVA, "drop_p", "DROP TABLE p;")
	add := pbBranch(root, pbVB, "add_c", "CREATE TABLE c (id bigint, p_id bigint);", pbTable("p"), child)
	_, err := migrate.Chain(pbMerge(t, drop, add))
	if !errors.Is(err, migrate.ErrBranchConflict) || !strings.Contains(err.Error(), "c.c_p") {
		t.Fatalf("want a conflict naming the foreign key, got %v", err)
	}
}

func pbDesired(tables ...string) *schema.Schema {
	s := &schema.Schema{Dialect: "postgres"}
	for _, n := range tables {
		s.Tables = append(s.Tables, schema.Table{Name: n, Columns: []schema.Column{{Name: "id", Type: "BIGINT"}}})
	}
	return s
}

// The next diff after a merge builds on the derived state: it adds the new
// table and does not revert the other branch's work.
func TestDiffAfterMergeBuildsOnDerivedState(t *testing.T) {
	merged := pbMergedIndependent(t)
	e := offlineEngine(t, merged, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))
	m, err := e.Diff(context.Background(), pbDesired("base", "a", "b", "c"), "add_c", migrate.DiffOptions{})
	if err != nil || m == nil {
		t.Fatalf("diff: %v %v", m, err)
	}
	if m.Parent != pbVB {
		t.Fatalf("parent = %s, want the latest version %s", m.Parent, pbVB)
	}
	var up string
	for name, data := range m.Files {
		if strings.HasSuffix(name, ".up.sql") {
			up = string(data)
		}
	}
	if !strings.Contains(up, "CREATE TABLE") || strings.Contains(up, "DROP") {
		t.Fatalf("the migration must only create c:\n%s", up)
	}
	next := withFiles(merged, m.Files)
	if p := pbProblems(t, next); len(p) != 0 {
		t.Fatalf("merged directory plus the new migration: %v", p)
	}
	if again, err := offlineEngine(t, next, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)).Diff(context.Background(), pbDesired("base", "a", "b", "c"), "again", migrate.DiffOptions{}); err != nil || again != nil {
		t.Fatalf("nothing left to diff: %+v %v", again, err)
	}
}

// A custom migration written after a merge copies the derived state, and
// validate compares it with the parent's derived state, not its file.
func TestCustomMigrationAfterMerge(t *testing.T) {
	merged := pbMergedIndependent(t)
	m, err := offlineEngine(t, merged, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)).NewCustom("backfill")
	if err != nil {
		t.Fatal(err)
	}
	snap, err := migrate.ReadSnapshot(bytes.NewReader(m.Files[m.Version+"_backfill.snapshot.json"]))
	if err != nil {
		t.Fatal(err)
	}
	if got := tableNames(snap.Schema); !sameStrings(got, []string{"a", "b", "base"}) || snap.ParentVersion != pbVB {
		t.Fatalf("custom snapshot = %v parent %s", got, snap.ParentVersion)
	}
	if p := pbProblems(t, withFiles(merged, m.Files)); len(p) != 0 {
		t.Fatalf("validate: %v", p)
	}
}

// pbApplyRenumber returns dir with the renumbered files written and the
// removed ones deleted, as the command does.
func pbApplyRenumber(dir fstest.MapFS, removed []string, files map[string][]byte) fstest.MapFS {
	out := withFiles(dir, files)
	for _, r := range removed {
		delete(out, r)
	}
	return out
}

// Branch A (version ...000000) was generated before branch B (...000500)
// reached the database. Renumber moves A after B so Apply stops refusing it.
func TestRenumberMovesPendingAfterNewestApplied(t *testing.T) {
	merged := pbMergedIndependent(t)
	e := offlineEngine(t, merged, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	removed, files, moved, err := e.Renumber([]string{pbV0, pbVB})
	if err != nil {
		t.Fatal(err)
	}
	if len(moved) != 1 || moved[0].Name != "add_a" || moved[0].Old != pbVA || moved[0].New != "20260102000501" {
		t.Fatalf("moved = %+v", moved)
	}
	if len(removed) != 2 || removed[0] != pbVA+"_add_a.snapshot.json" || removed[1] != pbVA+"_add_a.up.sql" {
		t.Fatalf("removed = %v", removed)
	}
	next := pbApplyRenumber(merged, removed, files)
	if _, ok := next["20260102000501_add_a.up.sql"]; !ok {
		t.Fatalf("the up file must move: %v", files)
	}
	if string(next["20260102000501_add_a.up.sql"].Data) != string(merged[pbVA+"_add_a.up.sql"].Data) {
		t.Fatal("the SQL must not change")
	}
	snap, err := migrate.ReadSnapshot(bytes.NewReader(next["20260102000501_add_a.snapshot.json"].Data))
	if err != nil || snap.Version != "20260102000501" || snap.ParentVersion != pbV0 {
		t.Fatalf("snapshot = %+v %v", snap, err)
	}
	if p := pbProblems(t, next); len(p) != 0 {
		t.Fatalf("renumbered directory: %v", p)
	}
	st, err := migrate.LatestState(next)
	if err != nil || !sameStrings(tableNames(st.Schema), []string{"a", "b", "base"}) {
		t.Fatalf("state after renumber: %v %v", st, err)
	}
}

// A pending migration built on a moved one moves with it, and its parent link
// follows.
func TestRenumberMovesPendingDescendants(t *testing.T) {
	merged := pbMergedIndependent(t)
	c := pbBranch(merged, "20260103000000", "add_c", "CREATE TABLE c (id bigint PRIMARY KEY);", pbTable("base"), pbTable("a"), pbTable("c"))
	c["20260103000000_add_c.snapshot.json"] = pbFile(pbSnapshot("20260103000000", pbVA, pbTable("base"), pbTable("a"), pbTable("c")))
	c = pbMerge(t, c, fstest.MapFS{})
	e := offlineEngine(t, c, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	removed, files, moved, err := e.Renumber([]string{pbV0, pbVB})
	if err != nil {
		t.Fatal(err)
	}
	if len(moved) != 2 || moved[0].Name != "add_a" || moved[1].Name != "add_c" {
		t.Fatalf("moved = %+v", moved)
	}
	next := pbApplyRenumber(c, removed, files)
	snap, err := migrate.ReadSnapshot(bytes.NewReader(next[moved[1].New+"_add_c.snapshot.json"].Data))
	if err != nil || snap.ParentVersion != moved[0].New {
		t.Fatalf("the moved child's parent must follow its parent: %+v %v", snap, err)
	}
	if p := pbProblems(t, next); len(p) != 0 {
		t.Fatalf("renumbered directory: %v", p)
	}
}

// Nothing moves when no pending migration is older than the newest applied
// one, and an applied migration is never moved.
func TestRenumberLeavesAppliedAndNewer(t *testing.T) {
	merged := pbMergedIndependent(t)
	e := offlineEngine(t, merged, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	for name, applied := range map[string][]string{
		"only the base applied":  {pbV0},
		"everything applied":     {pbV0, pbVA, pbVB},
		"nothing applied at all": nil,
		"pending are newer":      {pbV0, pbVA},
	} {
		removed, files, moved, err := e.Renumber(applied)
		if err != nil || len(removed) != 0 || len(files) != 0 || len(moved) != 0 {
			t.Errorf("%s: want nothing to do, got %v %v %v %v", name, removed, files, moved, err)
		}
	}
}

func TestRenumberRefusesAppliedChildOfPending(t *testing.T) {
	merged := pbMergedIndependent(t)
	c := pbBranch(merged, "20260103000000", "add_c", "CREATE TABLE c (id bigint PRIMARY KEY);", pbTable("base"), pbTable("a"), pbTable("c"))
	c["20260103000000_add_c.snapshot.json"] = pbFile(pbSnapshot("20260103000000", pbVA, pbTable("base"), pbTable("a"), pbTable("c")))
	e := offlineEngine(t, pbMerge(t, c, fstest.MapFS{}), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if _, _, _, err := e.Renumber([]string{pbV0, pbVB, "20260103000000"}); err == nil || !strings.Contains(err.Error(), "applied") {
		t.Fatalf("want a refusal of an applied child of a pending migration, got %v", err)
	}
}
