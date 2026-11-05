// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestColumnByName(t *testing.T) {
	dir, err := filepath.Abs("testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := Load([]string{"./sensitive"}, Config{Dir: dir})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	src, ds := EmitLookup(findModel(t, g, "Account"), g)
	if len(ds) != 0 {
		t.Fatalf("diags %v", ds)
	}
	s := string(src)
	for _, want := range []string{`case "email":`, `return Email, nil`, `momo.ErrValidation`, `func ColumnNames() []string`} {
		if !strings.Contains(s, want) {
			t.Fatalf("lookup source lacks %q", want)
		}
	}
	if strings.Contains(s, `case "password":`) || strings.Contains(s, `"password"`) {
		t.Fatal("sensitive column password is in the allowlist")
	}
}

func findModel(t *testing.T, g *Graph, name string) *Model {
	t.Helper()
	for _, m := range g.Models {
		if m.GoName == name {
			return m
		}
	}
	t.Fatalf("model %s missing", name)
	return nil
}
