// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"math"
	"strings"
	"testing"
)

// TestValidateSequence names each fault PostgreSQL would refuse and accepts the
// defaults and the extreme legal values.
//
// Catches: a bound check that compares before defaults are filled in, or one
// that rejects a legal sequence at the edge of its type.
func TestValidateSequence(t *testing.T) {
	bad := []struct {
		name string
		q    Sequence
		want string
	}{
		{"type", Sequence{Name: "s", AsType: "numeric"}, "is not smallint, integer or bigint"},
		{"min above max", Sequence{Name: "s", MinValue: Int64(9), MaxValue: Int64(3)}, "MinValue 9 is above MaxValue 3"},
		{"start below min", Sequence{Name: "s", MinValue: Int64(5), Start: Int64(4)}, "Start 4 is outside"},
		{"start above max", Sequence{Name: "s", MaxValue: Int64(5), Start: Int64(6)}, "Start 6 is outside"},
		{"max over type", Sequence{Name: "s", AsType: "smallint", MaxValue: Int64(32768)}, "MaxValue 32768 is outside the range of smallint"},
		{"min under type", Sequence{Name: "s", AsType: "int2", MinValue: Int64(-32769)}, "MinValue -32769 is outside the range of smallint"},
		{"descending default max", Sequence{Name: "s", Increment: -1, MinValue: Int64(0)}, "MinValue 0 is above MaxValue -1"},
	}
	for _, c := range bad {
		err := ValidateSequence(c.q)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.name, err, c.want)
		}
	}
	ok := []Sequence{
		{Name: "s"},
		{Name: "s", AsType: "INT4", Start: Int64(0), MinValue: Int64(0)},
		{Name: "s", AsType: "smallint", MinValue: Int64(math.MinInt16), MaxValue: Int64(math.MaxInt16), Start: Int64(0)},
		{Name: "s", Increment: -3, AsType: "bigint"},
		{Name: "s", MinValue: Int64(7), MaxValue: Int64(7)},
	}
	for _, q := range ok {
		if err := ValidateSequence(q); err != nil {
			t.Errorf("%+v: unexpected error %v", q, err)
		}
	}
}
