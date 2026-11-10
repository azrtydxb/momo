// SPDX-License-Identifier: Apache-2.0

package migrate_test

import (
	"bytes"
	"testing"
	"testing/fstest"
	"time"

	"github.com/azrtydxb/momo/migrate"
)

// customWith adds a custom migration at when whose up file holds body, and
// refreshes momo.sum so the directory stays valid.
func customWith(t *testing.T, dir fstest.MapFS, when time.Time, name, body string) string {
	t.Helper()
	m, err := offlineEngine(t, dir, when).NewCustom(name)
	if err != nil {
		t.Fatal(err)
	}
	for file, data := range m.Files {
		dir[file] = &fstest.MapFile{Data: data}
	}
	dir[m.Version+"_"+name+".up.sql"] = &fstest.MapFile{Data: []byte(body)}
	sum, err := migrate.ComputeSum(dir)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := sum.Write(&buf); err != nil {
		t.Fatal(err)
	}
	dir[migrate.SumFileName] = &fstest.MapFile{Data: buf.Bytes()}
	return m.Version
}

func TestEngineLintAllReadsTheDirectory(t *testing.T) {
	dir := initDir(t)
	risky := customWith(t, dir, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), "risky",
		"ALTER TABLE docs ADD COLUMN tenant bigint NOT NULL;\nCREATE INDEX docs_tenant ON docs (tenant);\n")
	reviewed := customWith(t, dir, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), "reviewed",
		"-- momo:destructive\nDROP TABLE docs;\n")
	unreviewed := customWith(t, dir, time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), "unreviewed",
		"DROP TABLE docs;\n")

	got, err := offlineEngine(t, dir, time.Now()).LintAll()
	if err != nil {
		t.Fatal(err)
	}
	byVersion := map[string][]string{}
	for _, f := range got {
		if f.Alternative == "" || f.Severity != migrate.SeverityError {
			t.Errorf("finding without alternative or error severity: %+v", f)
		}
		byVersion[f.Version] = append(byVersion[f.Version], f.Rule)
	}
	// The generated init migration creates its tables, so nothing in it is reported.
	if len(byVersion["20260102030405"]) != 0 {
		t.Errorf("init migration reported: %v", byVersion["20260102030405"])
	}
	if r := byVersion[risky]; len(r) != 2 || r[0] != migrate.RuleNotNullWithoutDefault || r[1] != migrate.RuleBlockingIndex {
		t.Errorf("risky: %v", r)
	}
	if r := byVersion[reviewed]; len(r) != 0 {
		t.Errorf("a -- momo:destructive migration must pass the destructive rule: %v", r)
	}
	if r := byVersion[unreviewed]; len(r) != 1 || r[0] != migrate.RuleDestructiveWithoutDirective {
		t.Errorf("unreviewed: %v", r)
	}
	if !migrate.HasErrors(got) {
		t.Error("HasErrors must be true")
	}
}
