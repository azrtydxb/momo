// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

// TestCompanionSchemaMethods loads a package whose models declare views, a
// function, a trigger, a policy, RLS and a partition through the structured
// companion methods, and checks they arrive in the built schema.
func TestCompanionSchemaMethods(t *testing.T) {
	g, diags, err := Load([]string{"./companionschema"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatalf("load diagnostics: %v", diags)
	}
	s, sdiags := BuildSchema(g.Models, Config{Dialects: []dialect.Name{dialect.Postgres}})
	if len(sdiags) != 0 {
		t.Fatalf("schema diagnostics: %v", sdiags)
	}
	tbl, ok := s.Table("docs")
	if !ok {
		t.Fatal("docs table missing")
	}
	if tbl.RLS == nil || !tbl.RLS.Enabled || !tbl.RLS.Forced {
		t.Fatalf("docs RLS = %+v", tbl.RLS)
	}
	if tbl.Partition == nil || tbl.Partition.Strategy != "range" || tbl.Partition.Key != "RANGE (id)" {
		t.Fatalf("docs partition = %+v", tbl.Partition)
	}
	if len(s.Policies) != 1 || s.Policies[0].Using == "" {
		t.Fatalf("policies = %+v", s.Policies)
	}
	if len(s.Functions) != 1 || s.Functions[0].Language != "plpgsql" {
		t.Fatalf("functions = %+v", s.Functions)
	}
	if len(s.Triggers) != 1 || s.Triggers[0].Function != "set_tenant" {
		t.Fatalf("triggers = %+v", s.Triggers)
	}
	if len(s.Views) != 1 || !s.Views[0].Materialized || s.Views[0].Name != "doc_view" {
		t.Fatalf("views = %+v", s.Views)
	}
}

// TestCompanionSchemaDiagnostics checks that an identifier or a non-literal
// value in a structured companion argument is diagnosed at the argument, and
// that no model is returned for the package.
func TestCompanionSchemaDiagnostics(t *testing.T) {
	dir := fixturesDir(t)
	g, diags, err := Load([]string{"./companionschemabad"}, Config{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(filepath.Join(dir, "companionschemabad", "bad.go")) //nolint:gosec // test path
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(src), "\n")
	got := diagStrings(diags)
	for n := 1; n <= 3; n++ {
		marker := fmt.Sprintf("// BAD-%d", n)
		line := -1
		for i, l := range lines {
			if strings.Contains(l, marker) {
				line = i + 1
			}
		}
		if line < 0 {
			t.Fatalf("fixture lacks %s", marker)
		}
		if want := fmt.Sprintf("bad.go:%d:", line); !strings.Contains(got, want) {
			t.Fatalf("missing diagnostic at %s:\n%s", want, got)
		}
	}
	if len(g.Models) != 0 {
		t.Fatalf("partial output: %d models", len(g.Models))
	}
}
