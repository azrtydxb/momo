// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"regexp"
	"strings"
)

// timeTypePattern is the allowlist for a Config.TypeDefaults value. The value
// is written into DDL, so only a date/time type with an optional precision and
// time zone clause is accepted.
var timeTypePattern = regexp.MustCompile(`^(timestamp|timestamptz|time|timetz)(\([0-6]\))?( with(out)? time zone)?$`)

// ValidTimeType reports whether s, after trimming, is an accepted value for
// the time.Time entry of Config.TypeDefaults (and of the types: section of
// momo.yaml): timestamp, timestamptz, time or timetz, an optional precision
// 0-6, and an optional with or without time zone clause.
func ValidTimeType(s string) bool {
	// strings.ToLower folds Unicode (the Kelvin sign and long s lowercase to
	// ASCII letters), so reject anything non-ASCII before matching.
	s = strings.TrimSpace(s)
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return timeTypePattern.MatchString(strings.ToLower(s))
}
