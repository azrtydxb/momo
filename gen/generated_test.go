// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"strings"
	"testing"
)

// Catches: the generated tag parsed but never reaching schema.Column, so the
// snapshot and migrate lose the GENERATED ALWAYS AS expression.
func TestGeneratedTagReachesColumn(t *testing.T) {
	norm := fld("Normalized", "string", "read_only,generated:lower(name)")
	norm.Access = ReadOnly
	s, diags := build(t, pg(), mdl("Thing", "things", fld("Name", "string", ""), norm))
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	col, ok := s.Tables[0].Column("normalized")
	if !ok || col.Generated != "lower(name)" {
		t.Fatalf("column = %+v, want the generated expression", col)
	}
}

// Catches: a generated column that is writable, which Insert and Update would
// write and the database would refuse, or one that also has a default.
func TestGeneratedTagRefusals(t *testing.T) {
	writable := fld("N", "string", "generated:lower(name)")
	withDefault := fld("N", "string", "read_only,generated:lower(name),default:'x'")
	withDefault.Access = ReadOnly
	for name, tc := range map[string]struct {
		f    *Field
		want string
	}{
		"writable":     {writable, "must be read_only"},
		"with default": {withDefault, "generated conflicts with default"},
	} {
		t.Run(name, func(t *testing.T) {
			_, diags := build(t, pg(), mdl("Thing", "things", tc.f))
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
