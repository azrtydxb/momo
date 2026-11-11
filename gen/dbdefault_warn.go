// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"fmt"
	"strings"

	"github.com/azrtydxb/momo/internal/dbdefault"
)

// dbDefaultWarnings reports the dbdefault fields that cannot store an explicit
// false or 0: a non-pointer bool or number field whose column default is a
// non-zero constant. The zero value means "use the database default" there, so
// Model replaces false and 0 with the default. A pointer field writes them.
// The default comes from the default tag or the MomoConfig Default entry.
func dbDefaultWarnings(models []*Model) []Diagnostic {
	var out []Diagnostic
	for _, m := range models {
		for _, f := range m.Fields {
			if !f.Tags.DBDefault || strings.HasPrefix(f.GoType, "*") {
				continue
			}
			base := f.BaseType
			if base == "" {
				base = f.GoType
			}
			def := f.Tags.Default
			if def == "" && m.Config != nil {
				def = m.Config.Defaults[f.Column]
			}
			if !dbdefault.NonZeroConst(base, def) {
				continue
			}
			out = append(out, Diagnostic{Pos: f.Pos, Msg: fmt.Sprintf(
				"field %s: dbdefault on a %s column whose default is %s: the zero value means \"use the default\", so an explicit false or 0 is replaced by it; make the field *%s to write it",
				f.GoName, f.GoType, strings.TrimSpace(def), f.GoType)})
		}
	}
	return out
}
