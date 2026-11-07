// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func TestSerialTagBuildsSerialColumns(t *testing.T) {
	m := mdl("Thing", "things",
		fld("ID", "int64", "pk,serial"),
		fld("Seq", "int32", "serial"),
		fld("Small", "int16", "serial"))
	s, diags := build(t, pg(), m)
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	for col, typ := range map[string]string{"id": "bigint", "seq": "integer", "small": "smallint"} {
		c := mustCol(t, s, "things", col)
		if c.Identity != "serial" || c.Type != typ || c.Default != "" {
			t.Errorf("%s = %+v, want serial %s with no default", col, c, typ)
		}
	}
}

func TestSerialTagRefusals(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		fld  *Field
		want string
	}{
		{"identity", pg(), fld("ID", "int64", "pk,serial,identity"), "serial conflicts with identity"},
		{"default", pg(), fld("ID", "int64", "pk,serial,default:5"), "serial conflicts with default"},
		{"pointer", pg(), fld("ID", "*int32", "serial"), "serial columns are always NOT NULL"},
		{"nullable tag", pg(), fld("ID", "int32", "serial,nullable"), "serial columns are always NOT NULL"},
		{"text", pg(), fld("ID", "string", "pk,serial"), "serial needs an integer column"},
		{"sqlite", Config{Dialects: []dialect.Name{dialect.SQLite}}, fld("ID", "int64", "pk,serial"), "serial is not supported on sqlite"},
		{"mysql", Config{Dialects: []dialect.Name{dialect.Postgres, dialect.MySQL}}, fld("ID", "int64", "pk,serial"), "serial is not supported on mysql"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, diags := build(t, tc.cfg, mdl("Thing", "things", tc.fld))
			var msgs []string
			for _, d := range diags {
				msgs = append(msgs, d.Msg)
			}
			if !strings.Contains(strings.Join(msgs, "\n"), tc.want) {
				t.Fatalf("diagnostics %q lack %q", msgs, tc.want)
			}
		})
	}
}

// Catches: either serial || identity condition in the emitters being dropped,
// which would leave a serial id without FlagIdentity or without the Meta
// Identity index, so inserts would write the key instead of reading it back.
func TestSerialEmitsIdentityFlagAndIndex(t *testing.T) {
	m := mdl("Thing", "things",
		fld("Name", "string", ""),
		fld("ID", "int64", "pk,serial"))
	if got := flagExpr(m.Fields[1]); got != "momo.FlagPK|momo.FlagIdentity" {
		t.Fatalf("flags = %q", got)
	}
	s, diags := build(t, pg(), m)
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	tb, _ := s.Table("things")
	var b strings.Builder
	emitModelMeta(&b, m, tb, "thing")
	out := b.String()
	if !strings.Contains(out, "Identity: 1,") {
		t.Fatalf("Meta Identity must be the serial field index 1:\n%s", out)
	}
	if !strings.Contains(out, `{Name: "id", Flags: momo.FlagPK|momo.FlagIdentity}`) {
		t.Fatalf("serial column lacks FlagIdentity:\n%s", out)
	}
}
