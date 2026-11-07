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
