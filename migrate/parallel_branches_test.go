// SPDX-License-Identifier: Apache-2.0

package migrate_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/azrtydxb/momo/migrate"
)

// The acceptance of docs/migrations.md "Parallel branches": two branches that
// each add a migration merge with git plus at most momo migrate hash, and the
// directory validates. The files below are what each branch's diff writes; the
// merged directory is their union, and its momo.sum is the union of the two
// branches' lines, as git's union merge driver leaves it.

const (
	pbV0 = "20260101000000"
	pbVA = "20260102000000"
	pbVB = "20260102000500"
)

// pbTable is the JSON of a table with an id and the given extra text columns.
func pbTable(name string, cols ...string) string {
	out := `{"name":"` + name + `","columns":[{"name":"id","type":"bigint"}`
	for _, c := range cols {
		out += `,{"name":"` + c + `","type":"text"}`
	}
	return out + `]}`
}

// pbSnapshot is a snapshot file whose schema holds the tables.
func pbSnapshot(version, parent string, tables ...string) string {
	return `{"format_version":1,"version":"` + version + `","parent":"` + parent +
		`","schema":{"format_version":1,"dialect":"postgres","tables":[` + strings.Join(tables, ",") + `]}}`
}

func pbFile(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }

// pbBase is the directory both branches start from: one migration that creates
// the table base.
func pbBase() fstest.MapFS {
	return fstest.MapFS{
		pbV0 + "_init.up.sql":        pbFile("-- momo:generated\nCREATE TABLE base (id bigint PRIMARY KEY);\n"),
		pbV0 + "_init.snapshot.json": pbFile(pbSnapshot(pbV0, "", pbTable("base"))),
	}
}

// pbBranch is base plus the migration a branch adds: its up file, and its
// snapshot, which holds the base tables and the branch's.
func pbBranch(base fstest.MapFS, version, name, up string, tables ...string) fstest.MapFS {
	out := fstest.MapFS{}
	for k, v := range base {
		out[k] = v
	}
	out[version+"_"+name+".up.sql"] = pbFile("-- momo:generated\n" + up + "\n")
	out[version+"_"+name+".snapshot.json"] = pbFile(pbSnapshot(version, pbV0, tables...))
	return out
}

// pbSumLines is the momo.sum lines of fsys, one "<sha256>  <name>" per file.
func pbSumLines(t *testing.T, fsys fstest.MapFS) []string {
	t.Helper()
	files := fstest.MapFS{}
	for k, v := range fsys {
		if k != migrate.SumFileName {
			files[k] = v
		}
	}
	s, err := migrate.ComputeSum(files)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for name, h := range s.Files {
		out = append(out, h+"  "+name)
	}
	slices.Sort(out)
	return out
}

// pbMerge is git merge of two branches that share base: the union of their
// files, and a momo.sum that is their lines, ours first, then theirs that ours
// lacks, which is what merge=union leaves.
func pbMerge(t *testing.T, ours, theirs fstest.MapFS) fstest.MapFS {
	t.Helper()
	merged := fstest.MapFS{}
	for _, side := range []fstest.MapFS{ours, theirs} {
		for k, v := range side {
			merged[k] = v
		}
	}
	lines := pbSumLines(t, ours)
	for _, l := range pbSumLines(t, theirs) {
		if !slices.Contains(lines, l) {
			lines = append(lines, l)
		}
	}
	merged[migrate.SumFileName] = pbFile("# momo.sum format=2\n" + strings.Join(lines, "\n") + "\n")
	return merged
}

// pbMergedIndependent is the merge of branch A (adds table a) and branch B
// (adds table b), both generated from the base.
func pbMergedIndependent(t *testing.T) fstest.MapFS {
	t.Helper()
	base := pbBase()
	a := pbBranch(base, pbVA, "add_a", "CREATE TABLE a (id bigint PRIMARY KEY);", pbTable("base"), pbTable("a"))
	b := pbBranch(base, pbVB, "add_b", "CREATE TABLE b (id bigint PRIMARY KEY);", pbTable("base"), pbTable("b"))
	return pbMerge(t, a, b)
}

func pbProblems(t *testing.T, fsys fstest.MapFS) []string {
	t.Helper()
	rep, err := migrate.ValidateOffline(fsys)
	if err != nil {
		t.Fatal(err)
	}
	return rep.Problems
}

// Acceptance: independent migrations from two branches validate after git
// merge alone, and after momo migrate hash.
func TestParallelBranchesMergeValidates(t *testing.T) {
	merged := pbMergedIndependent(t)
	if p := pbProblems(t, merged); len(p) != 0 {
		t.Fatalf("validate after git merge: %v", p)
	}
	if got, err := migrate.Chain(merged); err != nil || len(got) != 3 {
		t.Fatalf("chain after git merge: %v %v", got, err)
	}

	// momo migrate hash: ComputeSum, written as momo.sum.
	sum, err := migrate.ComputeSum(merged)
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	if err := sum.Write(&buf); err != nil {
		t.Fatal(err)
	}
	merged[migrate.SumFileName] = pbFile(buf.String())
	if p := pbProblems(t, merged); len(p) != 0 {
		t.Fatalf("validate after hash: %v", p)
	}
}

// Two branches that change the same table are still a conflict, and the error
// names the table and both migrations.
func TestParallelBranchesSameTableConflicts(t *testing.T) {
	base := pbBase()
	a := pbBranch(base, pbVA, "add_x", "ALTER TABLE base ADD COLUMN x text;", pbTable("base", "x"))
	b := pbBranch(base, pbVB, "add_y", "ALTER TABLE base ADD COLUMN y text;", pbTable("base", "y"))
	merged := pbMerge(t, a, b)

	_, err := migrate.Chain(merged)
	if !errors.Is(err, migrate.ErrBranchConflict) {
		t.Fatalf("want ErrBranchConflict, got %v", err)
	}
	for _, want := range []string{`table "base"`, pbVA + "_add_x.snapshot.json", pbVB + "_add_y.snapshot.json"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the error must name %s: %v", want, err)
		}
	}
	found := false
	for _, p := range pbProblems(t, merged) {
		found = found || (strings.Contains(p, `table "base"`) && strings.Contains(p, pbVA+"_add_x"))
	}
	if !found {
		t.Fatalf("validate must report the conflict naming the table: %v", pbProblems(t, merged))
	}
}
