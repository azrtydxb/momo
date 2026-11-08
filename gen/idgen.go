// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"fmt"
	"regexp"
)

// idGenName is the shape of a generator name: the built-ins (cuid, uuid, ulid)
// and names registered at runtime with momo.WithIDGenerator.
var idGenName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// idGenEntry is one column that takes its id from a Go-side generator.
type idGenEntry struct {
	Column int // index into m.Fields, which is the column index in Meta
	Name   string
}

// idGensOf returns the id generators of m, from the idgen tags and the IDGen
// calls of MomoConfig, in field order, with a diagnostic for each problem. A
// field declared both ways, a non-string field, an invalid name and a field
// that is already identity or serial are errors.
func idGensOf(m *Model) ([]idGenEntry, []Diagnostic) {
	var out []idGenEntry
	var diags []Diagnostic
	for i, f := range m.Fields {
		name := f.Tags.IDGen
		var cfg string
		if m.Config != nil {
			cfg = m.Config.IDGens[f.Column]
		}
		bad := func(format string, a ...any) {
			diags = append(diags, Diagnostic{Pos: f.Pos, Msg: fmt.Sprintf("field %s: %s", f.GoName, fmt.Sprintf(format, a...))})
		}
		switch {
		case name != "" && cfg != "":
			bad("idgen is declared by both the tag and MomoConfig")
			continue
		case name == "":
			name = cfg
		}
		if name == "" {
			continue
		}
		switch {
		case !idGenName.MatchString(name):
			bad("idgen %q must be a lowercase name such as cuid, uuid or ulid", name)
		case f.BaseType != "string":
			bad("idgen needs a string field, not %s", f.GoType)
		case f.Tags.Identity || f.Tags.Serial:
			bad("idgen cannot be combined with identity or serial")
		default:
			out = append(out, idGenEntry{Column: i, Name: name})
		}
	}
	return out, diags
}
