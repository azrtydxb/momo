// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"testing"

	"github.com/azrtydxb/momo"
)

// Catches: ValidTypeName accepting a type name that could carry SQL text into
// a CAST (a feature package relies on it to validate the type it writes), or
// rejecting a portable name such as numeric(10,2).
func TestValidTypeName(t *testing.T) {
	for _, ok := range []string{"bigint", "double precision", "numeric(10,2)", "varchar(20)", "signed", "timestamp with time zone"} {
		if err := momo.ValidTypeName(ok); err != nil {
			t.Errorf("ValidTypeName(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", "bigint; DROP TABLE x", "big)int", "a(", "numeric(x)", "numeric()", "x--", "a  b", "int(1,)", "(10)", "a$b", "int)(", `"q"`} {
		if err := momo.ValidTypeName(bad); err == nil {
			t.Errorf("ValidTypeName(%q) = nil, want an error", bad)
		}
	}
}
