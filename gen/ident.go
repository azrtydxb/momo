// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"fmt"
	"go/token"
	"strings"
)

// columnName is one persisted field's resulting column, input to
// checkColumnCollisions.
type columnName struct {
	Field  string
	Column string
	Pos    token.Position
}

// checkColumnCollisions reports persisted fields of one model that produce
// the same column name, in field order. The diagnostic is positioned at the
// second (later) field.
func checkColumnCollisions(cols []columnName) []Diagnostic {
	var diags []Diagnostic
	first := map[string]string{}
	for _, c := range cols {
		prev, dup := first[c.Column]
		if !dup {
			first[c.Column] = c.Field
			continue
		}
		diags = append(diags, Diagnostic{
			Pos: c.Pos,
			Msg: fmt.Sprintf("column %q is produced by fields %s and %s", c.Column, c.Field, prev),
		})
	}
	return diags
}

// checkModelIdentifiers reports models of one package whose names are equal
// under strings.ToLower. The diagnostic is positioned at the later model.
func checkModelIdentifiers(models []*Model) []Diagnostic {
	type key struct{ pkg, lower string }
	first := map[key]*Model{}
	var diags []Diagnostic
	for _, m := range models {
		k := key{m.PkgPath, strings.ToLower(m.GoName)}
		prev, dup := first[k]
		if !dup {
			first[k] = m
			continue
		}
		diags = append(diags, Diagnostic{
			Pos: m.Pos,
			Msg: fmt.Sprintf("model identifiers collide in package %s: %s, %s", m.PkgName, prev.GoName, m.GoName),
		})
	}
	return diags
}
