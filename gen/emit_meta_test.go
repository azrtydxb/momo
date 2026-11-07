// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"bytes"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/gen/testdata/metarun/models"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/golden"
)

func TestEmitMetaGolden(t *testing.T) {
	dir, err := filepath.Abs("testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := Load([]string{"./models"}, Config{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatalf("load diagnostics: %v", diags)
	}
	src, ds := EmitMeta(g.Models, g)
	if len(ds) != 0 {
		t.Fatalf("emit diagnostics: %v", ds)
	}
	formatted, err := format.Source(src)
	if err != nil {
		t.Fatalf("output is not valid Go: %v", err)
	}
	if !bytes.Equal(formatted, src) {
		t.Fatal("output is not gofmt-clean")
	}
	ok, err := golden.Diff("testdata/golden/meta/models/momo_gen.go", src, *update)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("meta output differs from golden; review and rerun with -update")
	}
	if !strings.Contains(string(src), "func (*Document) MomoMeta() *momo.Meta[Document]") {
		t.Fatal("Document MomoMeta missing")
	}
}

// TestEmitMetaBuilds proves the emitted file for the runtime fixture is
// current and compiles with the models it was generated from.
func TestEmitMetaBuilds(t *testing.T) {
	dir, err := filepath.Abs("testdata/metarun")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := Load([]string{"./models"}, Config{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatalf("load diagnostics: %v", diags)
	}
	src, ds := EmitMeta(g.Models, g)
	if len(ds) != 0 {
		t.Fatalf("emit diagnostics: %v", ds)
	}
	ok, err := golden.Diff("testdata/metarun/models/momo_gen.go", src, *update)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("generated fixture is stale; rerun with -update")
	}
	cmd := exec.Command("go", "build", "./testdata/metarun/...")
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build of generated meta: %v\n%s", err, out)
	}
}

// TestEmitMetaRuntime runs the runtime over the generated meta.
func TestEmitMetaRuntime(t *testing.T) {
	fd := fakedriver.New(dialect.Postgres)
	q, err := momo.Select[models.Account](momo.New(fd)).SQL()
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT "accounts"."id", "accounts"."name", "accounts"."bio", "accounts"."version", "accounts"."created_at", "accounts"."updated_at", "accounts"."deleted_at", "accounts"."home_street", "accounts"."home_city" FROM "accounts" WHERE "accounts"."deleted_at" IS NULL`
	if q.SQL != want {
		t.Fatalf("got  %s\nwant %s", q.SQL, want)
	}

	m := (*models.Account)(nil).MomoMeta()
	if m.Schema != "tenancy" || m.Table != "accounts" {
		t.Fatalf("table = %q.%q", m.Schema, m.Table)
	}
	if m.Identity != 0 || m.Version != 3 || m.CreatedAt != 4 || m.UpdatedAt != 5 || m.SoftDelete != 6 {
		t.Fatalf("derived indexes: %+v", m)
	}
	if len(m.PK) != 1 || m.PK[0] != 0 || m.TableSchema == nil || m.TableSchema.Name != "accounts" {
		t.Fatalf("pk/schema: %+v", m)
	}
	var a models.Account
	*m.Ptr(&a, 7).(*string) = "main st"
	*m.Ptr(&a, 0).(*int64) = 9
	if a.Home.Street != "main st" || a.ID != 9 || m.Ptr(&a, len(m.Columns)) != nil {
		t.Fatalf("Ptr: %+v", a)
	}
	if m.Value(&a, 0) != int64(9) || m.Value(&a, 7) != "main st" || m.Value(&a, len(m.Columns)) != nil {
		t.Fatal("Value mismatch")
	}
}

// TestEmitMetaSoftDeleteKind proves the generator carries the soft_delete
// tag into Meta.SoftDeleteKind, so unix and bool models filter and write
// their own marker instead of behaving as timestamps.
func TestEmitMetaSoftDeleteKind(t *testing.T) {
	if k := (*models.Account)(nil).MomoMeta().SoftDeleteKind; k != momo.SoftDeleteTimestamp {
		t.Fatalf("timestamp kind = %v", k)
	}
	if k := (*models.UnixDoc)(nil).MomoMeta().SoftDeleteKind; k != momo.SoftDeleteUnix {
		t.Fatalf("unix kind = %v", k)
	}
	if k := (*models.BoolDoc)(nil).MomoMeta().SoftDeleteKind; k != momo.SoftDeleteBool {
		t.Fatalf("bool kind = %v", k)
	}
	q, err := momo.Select[models.BoolDoc](momo.New(fakedriver.New(dialect.Postgres))).SQL()
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT "bool_docs"."id", "bool_docs"."deleted" FROM "bool_docs" WHERE "bool_docs"."deleted" = FALSE`
	if q.SQL != want {
		t.Fatalf("bool filter:\n got %s\nwant %s", q.SQL, want)
	}
}

func TestEmitMetaProjectionGolden(t *testing.T) {
	g, diags, err := Load([]string{"./projection"}, Config{Dir: fixturesDir(t)})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	if len(g.Projections) != 2 || len(g.Models) != 3 {
		t.Fatalf("models %d projections %d", len(g.Models), len(g.Projections))
	}
	if len(g.Schema.Tables) != 3 {
		t.Fatalf("a projection must not get a table: %+v", g.Schema.Tables)
	}
	src, ds := EmitMeta(g.Models, g)
	if len(ds) != 0 {
		t.Fatalf("emit diagnostics: %v", ds)
	}
	formatted, err := format.Source(src)
	if err != nil || !bytes.Equal(formatted, src) {
		t.Fatalf("output is not valid gofmt-clean Go: %v", err)
	}
	ok, err := golden.Diff("testdata/golden/meta/projection/momo_gen.go", src, *update)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("projection meta differs from golden; review and rerun with -update")
	}
	s := strings.Join(strings.Fields(string(src)), " ")
	for _, want := range []string{
		`{Name: "id"}, {Name: "title"}, {Name: "points"}, {Name: "made_at"}`,
		`{Name: "score"}, {Name: "articles"}, {Name: "http_code"}`,
		"Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, bad := range []string{"hidden", "Extra"} {
		if strings.Contains(s, bad) {
			t.Errorf("unexported or skipped field %s is mapped", bad)
		}
	}
}

func TestProjectionOnlyPackageEmits(t *testing.T) {
	g, diags, err := Load([]string{"./projectiononly"}, Config{Dir: fixturesDir(t)})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	w := NewWriter(WriterOptions{ModuleRoot: fixturesDir(t)})
	if err := Emit(g, w); err != nil {
		t.Fatal(err)
	}
	outs := w.Outputs()
	if len(outs) != 1 {
		t.Fatalf("outputs = %v", outs)
	}
	src := string(outs[0].Data)
	if strings.Contains(src, "schema") || !strings.Contains(src, "func (*Count) MomoMeta()") {
		t.Fatalf("unexpected output:\n%s", src)
	}
}

func TestProjectionUnsupportedFieldNamesStructAndField(t *testing.T) {
	_, diags, err := Load([]string{"./projectionbad"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 1 || !strings.Contains(diags[0].Msg, "projection Odd") || !strings.Contains(diags[0].Msg, "field Value") ||
		!strings.Contains(diags[0].Msg, "complex128") || diags[0].Pos.Line != 10 {
		t.Fatalf("diags: %v", diags)
	}
}

// TestDescriptorTableCarriesSoftDelete proves the generated Table names the
// soft-delete column and its kind, which is how a projection read from it is
// filtered.
func TestDescriptorTableCarriesSoftDelete(t *testing.T) {
	g, diags, err := Load([]string{"./projection"}, Config{Dir: fixturesDir(t)})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	want := map[string]string{
		"Widget":     `var Table = momo.TableRef{Name: "widgets", SoftDelete: "deleted_at"}`,
		"UnixWidget": `var Table = momo.TableRef{Name: "unix_widgets", SoftDelete: "gone_at", SoftDeleteKind: momo.SoftDeleteUnix}`,
		"BoolWidget": `var Table = momo.TableRef{Name: "bool_widgets", SoftDelete: "removed", SoftDeleteKind: momo.SoftDeleteBool}`,
	}
	for _, m := range g.Models {
		src, ds := EmitDescriptors(m, g)
		if len(ds) != 0 {
			t.Fatalf("emit diagnostics: %v", ds)
		}
		if !strings.Contains(string(src), want[m.GoName]) {
			t.Errorf("%s: Table lacks its soft-delete column and kind, want %s in:\n%s", m.GoName, want[m.GoName], src)
		}
	}
}
