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
		if !found || c.Type != want {
			t.Errorf("column %s = %q (found %v), want %q", col, c.Type, found, want)
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
