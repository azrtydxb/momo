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

// Int64 returns a pointer to v, for the optional numeric fields of Sequence.
func Int64(v int64) *int64 { return &v }

// orDefault returns a copy of p, or def when p is nil.
func orDefault(p *int64, def int64) *int64 {
	if p == nil {
		return Int64(def)
	}
	return Int64(*p)
}

// NormalizeSequence returns q with every field a declaration may leave unset
// filled in with the PostgreSQL default, so that a declaration and the
// sequence an inspector reads back for it are equal: the type is bigint, the
// increment 1, the cache 1, and the bounds and start follow from the type and
// the direction (an ascending sequence runs from 1 to the largest value of its
// type and starts at its minimum; a descending one runs from the smallest value
// of its type to -1 and starts at its maximum). An explicit 0 is kept. The
// result shares no pointer with q. It is idempotent.
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
	defMin, defMax := int64(1), hi
	if q.Increment < 0 {
		defMin, defMax = lo, -1
	}
	q.MinValue = orDefault(q.MinValue, defMin)
	q.MaxValue = orDefault(q.MaxValue, defMax)
	if q.Increment > 0 {
		q.Start = orDefault(q.Start, *q.MinValue)
	} else {
		q.Start = orDefault(q.Start, *q.MaxValue)
	}
	return q
}
