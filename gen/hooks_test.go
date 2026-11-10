// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/internal/golden"
)

func emitHooksFixture(t *testing.T) []byte {
	t.Helper()
	dir, err := filepath.Abs("testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := Load([]string{"./hooks"}, Config{Dir: dir})
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
	return src
}

func TestModelHookEmission(t *testing.T) {
	src := emitHooksFixture(t)
	ok, err := golden.Diff("testdata/golden/hooks/momo_gen.go", src, *update)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("hook metadata differs from golden; review and rerun with -update. Got:\n%s", src)
	}
	s := string(src)
	// The runtime hook takes (ctx, *T), so the model method is wrapped.
	want := "Hooks: momo.HookSet[Doc]{BeforeInsert: func(ctx context.Context, m *Doc) error { return m.BeforeInsert(ctx) }, " +
		"AfterLoad: func(ctx context.Context, m *Doc) error { return m.AfterLoad(ctx) }},"
	if !strings.Contains(s, want) {
		t.Fatalf("Doc hooks not emitted in the fixed order:\n%s", s)
	}
	if !strings.Contains(s, "\"context\"") {
		t.Fatalf("generated file lacks the context import:\n%s", s)
	}
}

func TestZeroHookSet(t *testing.T) {
	s := string(emitHooksFixture(t))
	start := strings.Index(s, "var plainMeta = momo.Meta[Plain]{")
	if start < 0 {
		t.Fatal("plainMeta literal missing")
	}
	end := strings.Index(s[start:], "\n}\n")
	if end < 0 {
		t.Fatal("plainMeta literal not closed")
	}
	if strings.Contains(s[start:start+end], "Hooks:") {
		t.Fatalf("model without hooks has a Hooks field:\n%s", s[start:start+end])
	}
}

func TestModelHookSignature(t *testing.T) {
	dir, err := filepath.Abs("testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	_, diags, err := Load([]string{"./hooksbad"}, Config{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, d := range diags {
		b.WriteString(d.String() + "\n")
	}
	if !strings.Contains(b.String(), "hook BeforeInsert on Doc must have signature func(context.Context) error") {
		t.Fatalf("missing signature diagnostic:\n%s", b.String())
	}
	if !strings.Contains(b.String(), "bad.go:16:") {
		t.Fatalf("diagnostic is not positioned at the method:\n%s", b.String())
	}
}
