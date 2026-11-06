// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func snap(version, parent string) string {
	return `{"format_version":1,"version":"` + version + `","parent":"` + parent + `","schema":{"format_version":1,"dialect":"postgres","tables":[]}}`
}

func snapTable(version, parent, table string) string {
	return `{"format_version":1,"version":"` + version + `","parent":"` + parent + `","schema":{"format_version":1,"dialect":"postgres","tables":[{"name":"` + table + `","columns":[{"name":"id","type":"bigint"}]}]}}`
}

func sealed(t *testing.T, m fstest.MapFS) fstest.MapFS {
	t.Helper()
	s, err := ComputeSum(m)
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	if err := s.Write(&sb); err != nil {
		t.Fatal(err)
	}
	m[SumFileName] = &fstest.MapFile{Data: []byte(sb.String())}
	return m
}

func problemsOf(t *testing.T, m fstest.MapFS) []string {
	t.Helper()
	rep, err := ValidateOffline(m)
	if err != nil {
		t.Fatal(err)
	}
	return rep.Problems
}

func wantProblem(t *testing.T, problems []string, parts ...string) {
	t.Helper()
	for _, p := range problems {
		ok := true
		for _, part := range parts {
			ok = ok && strings.Contains(p, part)
		}
		if ok {
			return
		}
	}
	t.Fatalf("no problem containing %q in %q", parts, problems)
}

func TestMigrationBranchConflict(t *testing.T) {
	fsys := fstest.MapFS{
		"20260101000000_root.snapshot.json": {Data: []byte(snap("20260101000000", ""))},
		"20260102000000_a.snapshot.json":    {Data: []byte(snap("20260102000000", "20260101000000"))},
		"20260102000001_b.snapshot.json":    {Data: []byte(snap("20260102000001", "20260101000000"))},
	}
	_, err := Chain(fsys)
	if !errors.Is(err, ErrBranchConflict) {
		t.Fatalf("want ErrBranchConflict, got %v", err)
	}
	for _, f := range []string{"20260102000000_a.snapshot.json", "20260102000001_b.snapshot.json"} {
		if !strings.Contains(err.Error(), f) {
			t.Fatalf("error must name %s: %v", f, err)
		}
	}
}

func TestValidateOffline(t *testing.T) {
	fsys := fstest.MapFS{
		"20260101000000_root.up.sql":        {Data: []byte("CREATE TABLE a (x int);")},
		"20260101000000_root.snapshot.json": {Data: []byte(snap("20260101000000", ""))},
	}
	rep, err := ValidateOffline(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Problems) != 1 || !strings.Contains(rep.Problems[0], "momo.sum") {
		t.Fatalf("want one problem naming momo.sum, got %v", rep.Problems)
	}
}

func goodChain() fstest.MapFS {
	return fstest.MapFS{
		"20260101000000_root.up.sql":        {Data: []byte("-- momo:generated\nCREATE TABLE a (id bigint);")},
		"20260101000000_root.snapshot.json": {Data: []byte(snapTable("20260101000000", "", "a"))},
		"20260102000000_next.up.sql":        {Data: []byte("-- momo:generated\nCREATE TABLE b (id bigint);")},
		"20260102000000_next.snapshot.json": {Data: []byte(snapTable("20260102000000", "20260101000000", "b"))},
	}
}

func TestValidateOfflineClean(t *testing.T) {
	if p := problemsOf(t, sealed(t, goodChain())); len(p) != 0 {
		t.Fatalf("clean chain reported %v", p)
	}
	got, err := Chain(goodChain())
	if err != nil || len(got) != 2 || got[0].Version != "20260101000000" || got[1].Version != "20260102000000" {
		t.Fatalf("chain order wrong: %v %v", got, err)
	}
}

func TestValidateOfflineMissingParent(t *testing.T) {
	m := goodChain()
	delete(m, "20260101000000_root.snapshot.json")
	_, err := Chain(m)
	if err == nil || !strings.Contains(err.Error(), "20260102000000_next.snapshot.json") || !strings.Contains(err.Error(), "20260101000000") {
		t.Fatalf("missing parent must name the file and parent: %v", err)
	}
	wantProblem(t, problemsOf(t, sealed(t, m)), "20260102000000_next.snapshot.json", "missing")
}

func TestValidateOfflineCycle(t *testing.T) {
	m := fstest.MapFS{
		"20260101000000_a.snapshot.json": {Data: []byte(snap("20260101000000", "20260102000000"))},
		"20260102000000_b.snapshot.json": {Data: []byte(snap("20260102000000", "20260101000000"))},
	}
	_, err := Chain(m)
	if err == nil || !strings.Contains(err.Error(), "cycle") ||
		!strings.Contains(err.Error(), "20260101000000_a.snapshot.json") || !strings.Contains(err.Error(), "20260102000000_b.snapshot.json") {
		t.Fatalf("cycle must be reported naming both files: %v", err)
	}
}

func TestValidateOfflineParentNotPrevious(t *testing.T) {
	// b names a later version as its parent and c names a: no fork, no
	// missing parent, but the order is wrong.
	m := fstest.MapFS{
		"20260101000000_a.snapshot.json": {Data: []byte(snap("20260101000000", ""))},
		"20260102000000_b.snapshot.json": {Data: []byte(snap("20260102000000", "20260103000000"))},
		"20260103000000_c.snapshot.json": {Data: []byte(snap("20260103000000", "20260101000000"))},
	}
	_, err := Chain(m)
	if err == nil || !strings.Contains(err.Error(), "20260102000000_b.snapshot.json") || !strings.Contains(err.Error(), "previous") {
		t.Fatalf("parent that is not the previous version must be reported: %v", err)
	}
}

func TestValidateOfflineGap(t *testing.T) {
	// c skips b: b has no child, c's parent is a, a has two children.
	m := goodChain()
	m["20260103000000_third.up.sql"] = &fstest.MapFile{Data: []byte("-- momo:generated\nCREATE TABLE c (id bigint);")}
	m["20260103000000_third.snapshot.json"] = &fstest.MapFile{Data: []byte(snapTable("20260103000000", "20260101000000", "c"))}
	wantProblem(t, problemsOf(t, sealed(t, m)), "20260103000000_third.snapshot.json")
}

func TestValidateOfflineSumMismatch(t *testing.T) {
	m := sealed(t, goodChain())
	m["20260101000000_root.up.sql"] = &fstest.MapFile{Data: []byte("-- momo:generated\nDROP TABLE a;")}
	wantProblem(t, problemsOf(t, m), "momo.sum", "20260101000000_root.up.sql")
}

func TestValidateOfflineMissingSnapshot(t *testing.T) {
	m := goodChain()
	delete(m, "20260102000000_next.snapshot.json")
	wantProblem(t, problemsOf(t, sealed(t, m)), "20260102000000_next.up.sql", "snapshot")
}

func TestValidateOfflineCustomSnapshotEqualsParent(t *testing.T) {
	m := goodChain()
	m["20260102000000_next.up.sql"] = &fstest.MapFile{Data: []byte("INSERT INTO a VALUES (1);")}
	wantProblem(t, problemsOf(t, sealed(t, m)), "20260102000000_next.snapshot.json", "custom")
	m["20260102000000_next.snapshot.json"] = &fstest.MapFile{Data: []byte(snapTable("20260102000000", "20260101000000", "a"))}
	if p := problemsOf(t, sealed(t, m)); len(p) != 0 {
		t.Fatalf("custom migration with unchanged snapshot is valid: %v", p)
	}
}

func TestValidateOfflineBadDirective(t *testing.T) {
	m := goodChain()
	m["20260101000000_root.up.sql"] = &fstest.MapFile{Data: []byte("-- momo:bogus\nCREATE TABLE a (id bigint);")}
	wantProblem(t, problemsOf(t, sealed(t, m)), "20260101000000_root.up.sql")
}

func TestValidateOfflineMalformedName(t *testing.T) {
	m := goodChain()
	m["20261301000000_x.up.sql"] = &fstest.MapFile{Data: []byte("SELECT 1;")}
	wantProblem(t, problemsOf(t, m), "20261301000000_x.up.sql")
}

func TestNextVersionUsesInjectedUTCClock(t *testing.T) {
	loc := time.FixedZone("X", 5*3600)
	now := func() time.Time { return time.Date(2026, 3, 4, 10, 0, 0, 0, loc) }
	got, err := NextVersion(now, fstest.MapFS{})
	if err != nil || got != "20260304050000" {
		t.Fatalf("want UTC 20260304050000, got %q %v", got, err)
	}
}

func TestNextVersionStaysAfterLatest(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	m := fstest.MapFS{"20270101000000_x.snapshot.json": {Data: []byte(snap("20270101000000", ""))}}
	got, err := NextVersion(now, m)
	if err != nil || got != "20270101000001" {
		t.Fatalf("version must follow the latest existing one, got %q %v", got, err)
	}
}
