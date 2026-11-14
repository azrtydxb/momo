// SPDX-License-Identifier: Apache-2.0

package tags

import (
	"reflect"
	"testing"
)

// Catches: the SQLite table options of the momo.Model marker (autoincrement,
// without_rowid, strict) being rejected as unknown tag options, or parsed into
// the wrong flag, so --from-db output for a table that uses them does not load.
func TestTableOptionsParse(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		flag string
	}{
		{"autoincrement", "Autoincrement"},
		{"without_rowid", "WithoutRowID"},
		{"strict", "Strict"},
	} {
		got, probs := Parse("table:t," + tc.raw)
		if len(probs) != 0 {
			t.Errorf("%q: problems %+v", tc.raw, probs)
			continue
		}
		v := reflect.ValueOf(got)
		for _, name := range []string{"Autoincrement", "WithoutRowID", "Strict"} {
			f := v.FieldByName(name)
			if want := name == tc.flag; !f.IsValid() || f.Bool() != want {
				t.Errorf("%q: %s = %v, want %v", tc.raw, name, f.IsValid() && f.Bool(), want)
			}
		}
	}
	if _, probs := Parse("strict:yes"); len(probs) != 1 {
		t.Errorf("a table option takes no value: %+v", probs)
	}
}
