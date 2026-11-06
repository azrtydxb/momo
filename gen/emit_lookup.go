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
	return emitLookup(m, fieldIdents(m, nil))
}

// fieldIdents returns the Go identifier of each field of m, in field order,
// as the descriptor package declares it. extra holds names that are already
// taken in that package, such as import aliases. EmitDescriptors and the
// lookup emitter both use it, so the names they write always agree.
func fieldIdents(m *Model, extra map[string]bool) []string {
	used := DescriptorReserved()
	for k := range extra {
		used[k] = true
	}
	ids := make([]string, len(m.Fields))
	for i, f := range m.Fields {
		ids[i] = Identifier(f.GoName, used)
		used[ids[i]] = true
	}
	return ids
}

// emitLookup renders ColumnByName for m, using ids (from fieldIdents) as the
// descriptor names.
func emitLookup(m *Model, ids []string) ([]byte, []Diagnostic) {
	var cases, names []string
	seen := map[string]bool{}
	var b strings.Builder
	for i, f := range m.Fields {
		id := ids[i]
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
