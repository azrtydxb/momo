// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestViewMarkerEmission checks that a model embedding momo.View gets
// `View: true` in its Meta, a table model does not, and the marker is not
// taken for a column.
func TestViewMarkerEmission(t *testing.T) {
	dir, err := filepath.Abs("testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := Load([]string{"./views"}, Config{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	src, ds := EmitMeta(g.Models, g)
	if len(ds) != 0 {
		t.Fatalf("emit diagnostics: %v", ds)
	}
	s := string(src)
	if n := strings.Count(s, "View: true,"); n != 1 {
		t.Fatalf("View: true appears %d times, want 1:\n%s", n, s)
	}
	view := metaLiteral(t, s, "var activeAccountMeta")
	if !strings.Contains(view, "View: true,") {
		t.Fatalf("view model lacks View: true:\n%s", view)
	}
	table := metaLiteral(t, s, "var accountMeta")
	if strings.Contains(table, "View:") {
		t.Fatalf("table model has View:\n%s", table)
	}
	if strings.Contains(view, `"view"`) {
		t.Fatalf("the View marker became a column:\n%s", view)
	}
}

// metaLiteral returns the Meta literal in s that starts at start.
func metaLiteral(t *testing.T, s, start string) string {
	t.Helper()
	_, rest, ok := strings.Cut(s, start)
	if !ok {
		t.Fatalf("%q missing in:\n%s", start, s)
	}
	lit, _, ok := strings.Cut(rest, "\n}\n")
	if !ok {
		t.Fatalf("%q literal not closed", start)
	}
	return lit
}

// TestProjectionHookDiagnostic checks that a hook-named method on a
// projection is reported instead of silently ignored.
func TestProjectionHookDiagnostic(t *testing.T) {
	dir, err := filepath.Abs("testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	_, diags, err := Load([]string{"./projhook"}, Config{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, d := range diags {
		b.WriteString(d.String() + "\n")
	}
	if !strings.Contains(b.String(), "AfterLoad on projection Card is never called") {
		t.Fatalf("missing projection hook diagnostic:\n%s", b.String())
	}
	if !strings.Contains(b.String(), "bad.go:17:") {
		t.Fatalf("diagnostic is not positioned at the method:\n%s", b.String())
	}
}
