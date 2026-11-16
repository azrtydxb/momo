// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"math"
	"strings"
)

// sequenceTypes maps the spellings of a sequence data type to the one the
// inspector reports.
var sequenceTypes = map[string]string{
	"smallint": "smallint", "int2": "smallint",
	"integer": "integer", "int": "integer", "int4": "integer",
	"bigint": "bigint", "int8": "bigint",
}

// sequenceRange returns the smallest and largest value of a sequence data type
// (already normalized).
func sequenceRange(asType string) (lo, hi int64) {
	switch asType {
	case "smallint":
		return math.MinInt16, math.MaxInt16
	case "integer":
		return math.MinInt32, math.MaxInt32
	}
	return math.MinInt64, math.MaxInt64
}

// NormalizeSequence returns q with every field a declaration may leave unset
// filled in with the PostgreSQL default, so that a declaration and the
// sequence an inspector reads back for it are equal: the type is bigint, the
// increment 1, the cache 1, and the bounds and start follow from the type and
// the direction (an ascending sequence runs from 1 to the largest value of its
// type and starts at its minimum; a descending one runs from the smallest value
// of its type to -1 and starts at its maximum). It is idempotent.
func NormalizeSequence(q Sequence) Sequence {
	t := strings.ToLower(strings.TrimSpace(q.AsType))
	if t == "" {
		t = "bigint"
	}
	if n, ok := sequenceTypes[t]; ok {
		t = n
	}
	q.AsType = t
	if q.Increment == 0 {
		q.Increment = 1
	}
	if q.Cache == 0 {
		q.Cache = 1
	}
	lo, hi := sequenceRange(t)
	if q.Increment > 0 {
		if q.MinValue == 0 {
			q.MinValue = 1
		}
		if q.MaxValue == 0 {
			q.MaxValue = hi
		}
		if q.Start == 0 {
			q.Start = q.MinValue
		}
		return q
	}
	if q.MinValue == 0 {
		q.MinValue = lo
	}
	if q.MaxValue == 0 {
		q.MaxValue = -1
	}
	if q.Start == 0 {
		q.Start = q.MaxValue
	}
	return q
}
