// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

// TestGoogleUUIDFieldsMapWithoutTypeTag pins the whole generator path for
// github.com/google/uuid: UUID, *UUID and []UUID map to uuid and uuid[] with
// no type tag, and the descriptors carry the google type.
//
// Catches: a uuid.UUID field resolved to its underlying [16]byte (no mapping),
// a pointer that loses its mapping, and a slice that needs a type tag.
func TestGoogleUUIDFieldsMapWithoutTypeTag(t *testing.T) {
	g, diags, err := Load([]string{"./googleuuid"}, Config{Dir: fixturesDir(t)})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	s, sdiags := BuildSchema(g.Models, Config{Dialects: []dialect.Name{dialect.Postgres}})
	if len(sdiags) != 0 {
		t.Fatalf("schema diags: %v", sdiags)
	}
	tbl, ok := s.Table("members")
	if !ok {
		t.Fatal("members table missing")
	}
	for col, want := range map[string]string{"id": "uuid", "owner_id": "uuid", "team_ids": "uuid[]"} {
		c, found := tbl.Column(col)
		if !found {
			t.Errorf("column %s missing; table has %+v", col, tbl.Columns)
			continue
		}
		if c.Type != want {
			t.Errorf("column %s = %q, want %q", col, c.Type, want)
		}
	}
	src, ds := EmitDescriptors(g.Models[0], g)
	if len(ds) != 0 {
		t.Fatalf("emit diagnostics: %v", ds)
	}
	flat := strings.Join(strings.Fields(string(src)), " ")
	for _, want := range []string{
		`momo.NewColumn[uuid.UUID](q, "id"`,
		`momo.NewColumn[[]uuid.UUID](q, "team_ids"`,
		`"github.com/google/uuid"`,
	} {
		if !strings.Contains(flat, want) {
			t.Errorf("descriptors lack %q:\n%s", want, src)
		}
	}
	_, sdiags = BuildSchema(g.Models, Config{Dialects: []dialect.Name{dialect.SQLite}})
	if !strings.Contains(sdiagsString(sdiags), "no SQL mapping on sqlite") {
		t.Fatalf("expected the []uuid.UUID field to be unsupported on sqlite, got %v", sdiags)
	}
}

// Catches: a UUID type of another package that happens to be named uuid being
// read as github.com/google/uuid. A slice of it spells like []uuid.UUID, so
// without the guard in baseType it would silently map to uuid[]; the scalar
// resolves to its underlying [16]byte and must stay unmapped too.
func TestForeignUUIDPackageIsNotMapped(t *testing.T) {
	_, diags, err := Load([]string{"./foreignuuid"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, d := range diags {
		got = append(got, d.String())
	}
	all := strings.Join(got, "\n")
	for _, field := range []string{"field V:", "field Vs:"} {
		if !strings.Contains(all, field) || !strings.Contains(all, "has no SQL mapping on postgres") {
			t.Errorf("expected an unmapped diagnostic for %s, got:\n%s", field, all)
		}
	}
}

// Catches: the mapping depending on how the package is spelled. An import
// alias (gu) and a type alias (type ID = gu.UUID) are the same type as
// uuid.UUID and map the same way.
func TestGoogleUUIDThroughAliases(t *testing.T) {
	g, diags, err := Load([]string{"./googleuuidalias"}, Config{Dir: fixturesDir(t)})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	s, sdiags := BuildSchema(g.Models, Config{Dialects: []dialect.Name{dialect.Postgres}})
	if len(sdiags) != 0 {
		t.Fatalf("schema diags: %v", sdiags)
	}
	tbl, ok := s.Table("items")
	if !ok {
		t.Fatal("items table missing")
	}
	for col, want := range map[string]string{"a": "uuid", "b": "uuid", "c": "uuid[]", "d": "uuid[]"} {
		c, found := tbl.Column(col)
		if !found {
			t.Errorf("column %s missing; table has %+v", col, tbl.Columns)
			continue
		}
		if c.Type != want {
			t.Errorf("column %s = %q, want %q", col, c.Type, want)
		}
	}
}
