// SPDX-License-Identifier: Apache-2.0

package dbdefault

import "testing"

// Catches: a constant default read wrongly, so a pointer is chosen where the
// zero value is harmless, or a pointer is missed where false/0 would be lost.
func TestNonZeroConst(t *testing.T) {
	cases := []struct {
		goType, def string
		want        bool
	}{
		{"bool", "true", true},
		{"bool", "TRUE", true},
		{"bool", "false", false},
		{"int64", "1", true},
		{"int32", "(5)", true},
		{"int32", "'7'::integer", true},
		{"int64", "-1", true},
		{"int64", "0", false},
		{"int64", "'0'::bigint", false},
		{"float64", "0.5", true},
		{"float64", "0.0", false},
		{"int64", "nextval('s'::regclass)", false},
		{"int64", "", false},
		{"string", "1", false},
		{"time.Time", "now()", false},
	}
	for _, tc := range cases {
		if got := NonZeroConst(tc.goType, tc.def); got != tc.want {
			t.Errorf("NonZeroConst(%q, %q) = %v, want %v", tc.goType, tc.def, got, tc.want)
		}
	}
}
