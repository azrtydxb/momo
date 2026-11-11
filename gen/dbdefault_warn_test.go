// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"strings"
	"testing"
)

// Catches: no warning for a non-pointer dbdefault bool or number whose
// constant default is non-zero (an explicit false or 0 is replaced by it), or
// a warning where nothing is lost (a pointer, a zero default, a computed
// default, a string).
func TestDBDefaultZeroWarnings(t *testing.T) {
	m := mdl("Thing", "things",
		fld("A", "bool", "default:true,dbdefault"),
		fld("B", "int32", "default:5,dbdefault"),
		fld("C", "*bool", "default:true,dbdefault"),
		fld("D", "bool", "default:false,dbdefault"),
		fld("E", "int64", "default:0,dbdefault"),
		fld("F", "int64", "default:nextval('s'),dbdefault"),
		fld("G", "string", "default:'x',dbdefault"),
		fld("H", "bool", "default:true"),
		fld("I", "float64", "dbdefault"))
	m.Config.Defaults["i"] = "1.5"
	var got []string
	for _, d := range dbDefaultWarnings([]*Model{m}) {
		got = append(got, d.Msg[:strings.Index(d.Msg, ":")])
	}
	if want := []string{"field A", "field B", "field I"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("warnings for %v, want %v", got, want)
	}
}
