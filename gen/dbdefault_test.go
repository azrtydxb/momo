// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"strings"
	"testing"
)

// Catches: the dbdefault tag being parsed but never reaching the column
// flags, so Insert.Model keeps writing the Go zero value over the default.
func TestDBDefaultTagEmitsFlag(t *testing.T) {
	m := mdl("Event", "events",
		fld("ID", "string", "pk,type:uuid,default:gen_random_uuid(),dbdefault"),
		fld("Title", "string", ""))
	if got := flagExpr(m.Fields[0]); got != "momo.FlagPK|momo.FlagHasDefault|momo.FlagDBDefault" {
		t.Fatalf("flags = %q", got)
	}
	if got := flagExpr(m.Fields[1]); got != "0" {
		t.Fatalf("a field without the tag has flags %q", got)
	}
	_, diags := build(t, pg(), m)
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
}

// Catches: dbdefault silently accepted next to a tag that already decides how
// the column is written, which would give two owners to one value.
func TestDBDefaultTagRefusals(t *testing.T) {
	readOnly := fld("A", "string", "dbdefault")
	readOnly.Access = ReadOnly
	cases := []struct {
		name string
		fld  *Field
		want string
	}{
		{"identity", fld("ID", "int64", "pk,identity,dbdefault"), "dbdefault conflicts with identity"},
		{"serial", fld("ID", "int64", "pk,serial,dbdefault"), "dbdefault conflicts with serial"},
		{"idgen", fld("ID", "string", "pk,idgen:cuid,dbdefault"), "dbdefault conflicts with idgen"},
		{"version", fld("V", "int64", "version,dbdefault"), "dbdefault conflicts with version"},
		{"created_at", fld("C", "time.Time", "created_at,dbdefault"), "dbdefault conflicts with created_at"},
		{"updated_at", fld("U", "time.Time", "updated_at,dbdefault"), "dbdefault conflicts with updated_at"},
		{"soft_delete", fld("D", "*time.Time", "soft_delete:timestamp,dbdefault"), "dbdefault conflicts with soft_delete"},
		{"read_only", readOnly, "dbdefault conflicts with read_only"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, diags := build(t, pg(), mdl("Thing", "things", tc.fld))
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
