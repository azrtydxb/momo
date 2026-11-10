// SPDX-License-Identifier: Apache-2.0

package momotest

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// recTB records failures instead of failing the real test.
type recTB struct {
	testing.TB
	errs []string
}

func (r *recTB) Helper()                   {}
func (r *recTB) Errorf(f string, a ...any) { r.errs = append(r.errs, fmt.Sprintf(f, a...)) }
func (r *recTB) Fatalf(f string, a ...any) {
	r.errs = append(r.errs, fmt.Sprintf(f, a...))
	runtime.Goexit()
}
func (r *recTB) failed() bool { return len(r.errs) > 0 }
func (r *recTB) text() string { return strings.Join(r.errs, "\n") }

func registryWith(sql ...string) *momo.QueryRegistry {
	r := momo.NewQueryRegistry()
	for i, s := range sql {
		var args []any
		if strings.Contains(s, "$1") {
			args = []any{1}
		}
		r.Register(fmt.Sprintf("q%d", i+1), func(q momo.Querier) momo.Compilable { return momo.RawQuery(q, s, args...) })
	}
	return r
}

// Catches: a golden check that passes on changed SQL, or an update mode that
// does not write what the check then accepts.
func TestQueryGoldenDetectsChanges(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(UpdateGoldenEnv, "1")
	tb := &recTB{}
	run(func() { QueryGolden(tb, registryWith("SELECT 1", "SELECT a FROM t WHERE id = $1"), dir) })
	if tb.failed() {
		t.Fatalf("update mode failed: %s", tb.text())
	}
	b, err := os.ReadFile(filepath.Join(dir, "postgres.sql")) // #nosec G304 -- a path under t.TempDir
	if err != nil || string(b) != "-- name: q1\nSELECT 1;\n\n-- name: q2\nSELECT a FROM t WHERE id = $1;\n\n" {
		t.Fatalf("golden file = %q, %v", b, err)
	}

	t.Setenv(UpdateGoldenEnv, "")
	tb = &recTB{}
	run(func() { QueryGolden(tb, registryWith("SELECT 1", "SELECT a FROM t WHERE id = $1"), dir) })
	if tb.failed() {
		t.Fatalf("unchanged SQL failed: %s", tb.text())
	}

	for want, reg := range map[string]*momo.QueryRegistry{
		"~ changed q2": registryWith("SELECT 1", "SELECT b FROM t WHERE id = $1"),
		"- removed q2": registryWith("SELECT 1"),
		"+ added q3":   registryWith("SELECT 1", "SELECT a FROM t WHERE id = $1", "SELECT 3"),
	} {
		tb = &recTB{}
		run(func() { QueryGolden(tb, reg, dir) })
		if !tb.failed() || !strings.Contains(tb.text(), want) {
			t.Errorf("errors = %q, want a report containing %q", tb.text(), want)
		}
	}

	tb = &recTB{}
	run(func() { QueryGolden(tb, registryWith("SELECT 1"), dir, dialect.SQLite) })
	if !tb.failed() || !strings.Contains(tb.text(), UpdateGoldenEnv) {
		t.Errorf("a missing golden file: errors = %q, want a hint about %s", tb.text(), UpdateGoldenEnv)
	}
}

// Catches: a golden file the previous release wrote not parsing back.
func TestParseQueryGoldenRoundTrip(t *testing.T) {
	in := []momo.RegisteredQuery{
		{Name: "a", SQL: "SELECT 1"},
		{Name: "b", SQL: "SELECT x\nFROM t\nWHERE y = $1"},
	}
	out, err := ParseQueryGolden(FormatQueryGolden(in))
	if err != nil || len(out) != 2 || out[0] != in[0] || out[1] != in[1] {
		t.Fatalf("round trip = %+v, %v", out, err)
	}
	if out, err := ParseQueryGolden(""); err != nil || len(out) != 0 {
		t.Fatalf("empty = %+v, %v", out, err)
	}
}
