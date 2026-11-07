// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

// TestSliceFieldsMapToArrayColumns pins the whole generator path for scalar
// slices: the schema carries the PostgreSQL array type and the descriptor is
// a plain column of the slice type.
func TestSliceFieldsMapToArrayColumns(t *testing.T) {
	g, diags, err := Load([]string{"./arrays"}, Config{Dir: fixturesDir(t)})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	s, sdiags := BuildSchema(g.Models, Config{Dialects: []dialect.Name{dialect.Postgres}})
	if len(sdiags) != 0 {
		t.Fatalf("schema diags: %v", sdiags)
	}
	tbl, ok := s.Table("contacts")
	if !ok {
		t.Fatal("contacts table missing")
	}
	for col, want := range map[string]string{
		"products": "text[]", "scores": "integer[]", "counts": "bigint[]",
		"flags": "boolean[]", "weights": "double precision[]", "aliases": "text[]",
	} {
		c, found := tbl.Column(col)
		if !found || c.Type != want {
			t.Errorf("column %s = %q (found %v), want %q", col, c.Type, found, want)
		}
	}
	if c, _ := tbl.Column("aliases"); !c.Nullable {
		t.Error("aliases must be nullable: no notnull tag")
	}
	src, ds := EmitDescriptors(g.Models[0], g)
	if len(ds) != 0 {
		t.Fatalf("emit diagnostics: %v", ds)
	}
	flat := strings.Join(strings.Fields(string(src)), " ")
	for _, want := range []string{
		`Products momo.Column[[]string]`,
		`momo.NewColumn[[]string](q, "products"`,
		`momo.NewColumn[[]int32](q, "scores"`,
		`momo.NewColumn[[]float64](q, "weights"`,
	} {
		if !strings.Contains(flat, want) {
			t.Errorf("descriptors lack %q:\n%s", want, src)
		}
	}
	_, sdiags = BuildSchema(g.Models, Config{Dialects: []dialect.Name{dialect.SQLite}})
	if !strings.Contains(sdiagsString(sdiags), "no SQL mapping on sqlite") {
		t.Fatalf("expected an unsupported-type diagnostic on sqlite, got %v", sdiags)
	}
}
