// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"fmt"
	"strconv"
	"strings"
)

// EmitLookup returns the source of the allowlisted ColumnByName and
// ColumnNames functions of one model's descriptor subpackage (no header, no
// imports; EmitDescriptors appends it and records the "fmt" import).
//
// The allowlist is the persisted columns in Fields order minus sensitive
// fields. Matching is an exact string switch over database column names, so no
// input is ever interpolated into SQL and case variants, quoted or qualified
// names and the empty string are rejected.
func EmitLookup(m *Model, g *Graph) ([]byte, []Diagnostic) {
	_ = g
	used := DescriptorReserved()
	var cases, names []string
	seen := map[string]bool{}
	var b strings.Builder
	for _, f := range m.Fields {
		// Identifier must be consulted for every field, in the same order as
		// EmitDescriptors, so the Go names agree.
		id := Identifier(f.GoName, used)
		used[id] = true
		if f.Tags.Sensitive || seen[f.Column] {
			continue
		}
		seen[f.Column] = true
		q := strconv.Quote(f.Column)
		cases = append(cases, "\tcase "+q+":\n\t\treturn "+id+", nil\n")
		names = append(names, q)
	}

	b.WriteString("\n// ColumnByName returns the typed column for an allowlisted external name.\n")
	b.WriteString("// Names are matched exactly against database column names; relation fields and\n")
	b.WriteString("// sensitive fields are not in the allowlist.\n")
	b.WriteString("func ColumnByName(name string) (momo.ColumnRef, error) {\n")
	if len(cases) > 0 {
		b.WriteString("\tswitch name {\n")
		for _, c := range cases {
			b.WriteString(c)
		}
		b.WriteString("\t}\n")
	}
	fmt.Fprintf(&b, "\treturn nil, fmt.Errorf(\"%%w: column %%q is not allowed on %%s\", momo.ErrValidation, name, %s)\n}\n\n", strconv.Quote(m.Table))
	b.WriteString("// ColumnNames lists the allowlisted names in declaration order.\n")
	fmt.Fprintf(&b, "func ColumnNames() []string { return []string{%s} }\n", strings.Join(names, ", "))
	return []byte(b.String()), nil
}
