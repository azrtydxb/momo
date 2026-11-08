// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"regexp"
	"strings"
)

// timeTypePattern is the allowlist for a Config.TypeDefaults value. The value
// is written into DDL, so only a date/time type with an optional precision and
// time zone clause is accepted.
var timeTypePattern = regexp.MustCompile(`^(?i:(timestamp|timestamptz|time|timetz)(\([0-6]\))?( with(out)? time zone)?)$`)

// ValidTimeType reports whether s, after trimming, is an accepted value for
// the time.Time entry of Config.TypeDefaults (and of the types: section of
// momo.yaml): timestamp, timestamptz, time or timetz, an optional precision
// 0-6, and an optional with or without time zone clause.
func ValidTimeType(s string) bool {
	return timeTypePattern.MatchString(strings.TrimSpace(s))
}
