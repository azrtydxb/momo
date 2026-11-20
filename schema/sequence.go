// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"fmt"
	"math"
	"strings"

	"github.com/azrtydxb/momo/internal/schemautil"
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

// ValidateSequence reports why PostgreSQL would refuse CREATE SEQUENCE for q:
// a data type other than smallint, integer or bigint, a bound outside the
// range of the type, a minimum above the maximum, or a start outside the
// bounds. Unset fields take the defaults NormalizeSequence documents. An
// Increment of 0 reads as unset here, so a caller that must refuse an explicit
// 0 checks for it first.
func ValidateSequence(q Sequence) error {
	t := strings.ToLower(strings.TrimSpace(q.AsType))
	if _, ok := sequenceTypes[t]; t != "" && !ok {
		return fmt.Errorf("sequence %q: the data type %q is not smallint, integer or bigint", q.Name, q.AsType)
	}
	n := NormalizeSequence(q)
	lo, hi := sequenceRange(n.AsType)
	minV, maxV, start := *n.MinValue, *n.MaxValue, *n.Start
	if q.MinValue != nil && (minV < lo || minV > hi) {
		return fmt.Errorf("sequence %q: MinValue %d is outside the range of %s (%d to %d)", q.Name, minV, n.AsType, lo, hi)
	}
	if q.MaxValue != nil && (maxV < lo || maxV > hi) {
		return fmt.Errorf("sequence %q: MaxValue %d is outside the range of %s (%d to %d)", q.Name, maxV, n.AsType, lo, hi)
	}
	if minV > maxV {
		return fmt.Errorf("sequence %q: MinValue %d is above MaxValue %d", q.Name, minV, maxV)
	}
	if start < minV || start > maxV {
		return fmt.Errorf("sequence %q: Start %d is outside MinValue %d and MaxValue %d", q.Name, start, minV, maxV)
	}
	return nil
}

// orDefault returns a copy of p, or def when p is nil.
func orDefault(p *int64, def int64) *int64 {
	if p == nil {
		return schemautil.Int64(def)
	}
	return schemautil.Int64(*p)
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
