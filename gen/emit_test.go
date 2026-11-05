// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"bytes"
	"errors"
	"go/format"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGenerateDeterministic(t *testing.T) {
	dir, err := filepath.Abs("testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := Load([]string{"./models"}, Config{Dir: dir})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	run := func() []Output {
		w := NewWriter(WriterOptions{ModuleRoot: dir})
		if err := Emit(g, w); err != nil {
			t.Fatal(err)
		}
		return w.Outputs()
	}
	first, second := run(), run()
	if len(first) == 0 {
		t.Fatal("no files emitted")
	}
	if len(first) != len(second) {
		t.Fatalf("file counts differ: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i].Path != second[i].Path || !bytes.Equal(first[i].Data, second[i].Data) {
			t.Fatalf("%s differs between runs", first[i].Path)
		}
		formatted, err := format.Source(first[i].Data)
		if err != nil || !bytes.Equal(formatted, first[i].Data) {
			t.Fatalf("%s is not gofmt-clean", first[i].Path)
		}
	}
}

// TestGenerateDeterministicAcrossLoads reloads the graph for every run and
// varies GOMAXPROCS, so map iteration order and scheduling cannot hide.
func TestGenerateDeterministicAcrossLoads(t *testing.T) {
	dir, err := filepath.Abs("testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	prev := runtime.GOMAXPROCS(0)
	defer runtime.GOMAXPROCS(prev)
	var base []Output
	for i, procs := range []int{1, 4, 1, 8} {
		runtime.GOMAXPROCS(procs)
		g, diags, err := Load([]string{"./models", "./rel"}, Config{Dir: dir})
		if err != nil || len(diags) != 0 {
			t.Fatalf("load: %v %v", err, diags)
		}
		w := NewWriter(WriterOptions{ModuleRoot: dir})
		if err := Emit(g, w); err != nil {
			t.Fatal(err)
		}
		out := w.Outputs()
		if i == 0 {
			base = out
			continue
		}
		if len(out) != len(base) {
			t.Fatalf("GOMAXPROCS=%d: %d files, want %d", procs, len(out), len(base))
		}
		for j := range out {
			if out[j].Path != base[j].Path || !bytes.Equal(out[j].Data, base[j].Data) {
				t.Fatalf("GOMAXPROCS=%d: %s differs", procs, out[j].Path)
			}
		}
	}
}

func TestEmitOutputsSortedAndLayout(t *testing.T) {
	dir, err := filepath.Abs("testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	g, _, err := Load([]string{"./models", "./rel"}, Config{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	w := NewWriter(WriterOptions{ModuleRoot: dir})
	if err := Emit(g, w); err != nil {
		t.Fatal(err)
	}
	outs := w.Outputs()
	want := map[string]bool{}
	for _, m := range g.Models {
		want[filepath.Join(m.PkgDir, "momo_gen.go")] = true
		want[filepath.Join(m.PkgDir, m.SubPkg, "momo_gen.go")] = true
	}
	for i, o := range outs {
		if i > 0 && outs[i-1].Path >= o.Path {
			t.Fatalf("outputs not sorted: %s then %s", outs[i-1].Path, o.Path)
		}
		delete(want, o.Path)
	}
	if len(want) != 0 {
		t.Fatalf("missing outputs: %v", want)
	}
}

type recordPlugin struct {
	name  string
	order *[]string
}

func (p recordPlugin) Name() string { return p.name }
func (p recordPlugin) Generate(_ *Graph, _ *Writer) error {
	*p.order = append(*p.order, p.name)
	if p.name == "bad" {
		return errors.New("boom")
	}
	return nil
}

func TestEmitRunsPluginsInOrder(t *testing.T) {
	dir, err := filepath.Abs("testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	g, _, err := Load([]string{"./models"}, Config{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	g.Plugins = []Plugin{recordPlugin{"b", &order}, recordPlugin{"a", &order}, recordPlugin{"bad", &order}, recordPlugin{"never", &order}}
	err = Emit(g, NewWriter(WriterOptions{ModuleRoot: dir}))
	if err == nil || !strings.Contains(err.Error(), "plugin bad") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("want wrapped plugin error, got %v", err)
	}
	if got := strings.Join(order, ","); got != "b,a,bad" {
		t.Fatalf("plugin order %q", got)
	}
}

func TestEmitDiagnosticsWriteNothing(t *testing.T) {
	dir, err := filepath.Abs("testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	g, _, err := Load([]string{"./models"}, Config{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	// Without schema tables EmitMeta reports a diagnostic for every model.
	g.Schema.Tables = nil
	w := NewWriter(WriterOptions{ModuleRoot: dir})
	err = Emit(g, w)
	var de *ErrDiagnostics
	if !errors.As(err, &de) || len(de.Diagnostics) == 0 {
		t.Fatalf("want ErrDiagnostics, got %v", err)
	}
	if !strings.Contains(err.Error(), de.Diagnostics[0].String()) {
		t.Fatalf("error does not list diagnostics: %v", err)
	}
	if n := len(w.Outputs()); n != 0 {
		t.Fatalf("%d outputs buffered despite diagnostics", n)
	}
}
