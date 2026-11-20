// SPDX-License-Identifier: Apache-2.0

package genutil

import "testing"

// TestValidTimeType catches an allowlist that lets a value with SQL syntax
// through, or rejects a legitimate time type.
func TestValidTimeType(t *testing.T) {
	for _, ok := range []string{"timestamp", "timestamp(3)", " TIMESTAMPTZ(6) ", "timestamp(0) without time zone", "timestamp with time zone", "time(2)", "timetz"} {
		if !ValidTimeType(ok) {
			t.Errorf("ValidTimeType(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"", "text", "timestamp(3); DROP TABLE x", "timestamp'", `"timestamp"`, "timestamp(7)", "timestamp(3) --", "date", "timestamp(3)\ntext", "timeſtamp", "TIMEſTAMPTZ"} {
		if ValidTimeType(bad) {
			t.Errorf("ValidTimeType(%q) = true, want false", bad)
		}
	}
}
