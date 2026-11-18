// SPDX-License-Identifier: Apache-2.0

package types

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/azrtydxb/momo/dialect"
)

// RangeElem is the element types a Range holds, one for each built-in
// PostgreSQL range type:
//
//	int32           int4range
//	int64           int8range
//	Numeric         numrange
//	Date            daterange
//	Timestamp       tsrange     (a timestamp without a time zone)
//	time.Time       tstzrange   (a timestamp with a time zone)
type RangeElem interface {
	int32 | int64 | Numeric | Date | Timestamp | time.Time
}

// Timestamp is a timestamp without a time zone: timestamp on PostgreSQL. It is
// the element of a tsrange; use time.Time for a timestamp with a time zone.
//
// Value writes the wall clock of Time in its own location ("2026-01-01
// 10:00:00"), without a zone, which is what the database stores. Scan reads the
// wall clock back as a UTC time of the same fields, since the database keeps no
// zone: two Timestamps are equal when their wall clocks are, and a reader that
// wants an instant must apply its zone itself. Timestamp has no infinity or
// -infinity.
type Timestamp struct{ Time time.Time }

// timestampLayout is the text of a timestamp without a time zone, with an
// optional fraction of up to six digits (PostgreSQL's precision).
const timestampLayout = "2006-01-02 15:04:05.999999"

// The spellings of a timestamptz that PostgreSQL prints, by the length of the
// zone offset: +00, +05:30 and +05:30:15. The fraction is optional when
// parsing; zoneMinute is also the one Value writes.
const (
	zoneHour   = "2006-01-02 15:04:05.999999Z07"
	zoneMinute = "2006-01-02 15:04:05.999999Z07:00"
	zoneSecond = "2006-01-02 15:04:05.999999Z07:00:00"
)

// SQLType returns the column type for d.
func (Timestamp) SQLType(d dialect.Name) string {
	if d == dialect.Postgres {
		return "timestamp"
	}
	return "TEXT"
}

// Value encodes the wall clock of t as "YYYY-MM-DD HH:MM:SS[.ffffff]".
func (t Timestamp) Value() (driver.Value, error) {
	return t.Time.Format(timestampLayout), nil
}

// Scan accepts the text form as string or []byte, or a time.Time (as pgx
// returns for a timestamp column), whose wall clock is kept as UTC.
func (t *Timestamp) Scan(src any) error {
	if v, ok := src.(time.Time); ok {
		y, mo, d := v.Date()
		h, mi, s := v.Clock()
		*t = Timestamp{Time: time.Date(y, mo, d, h, mi, s, v.Nanosecond(), time.UTC)}
		return nil
	}
	s, err := text("Timestamp", src)
	if err != nil {
		return err
	}
	v, err := time.Parse("2006-01-02 15:04:05", strings.TrimSpace(s))
	if err != nil {
		return fmt.Errorf("types: invalid Timestamp %q: %w", s, err)
	}
	*t = Timestamp{Time: v}
	return nil
}

// Bound is one end of a Range. An Infinite bound is unbounded: Value and
// Inclusive mean nothing then, and a Range reads back from the database with
// both zero. A finite bound is Inclusive for [ or ] and exclusive for ( or ).
type Bound[T RangeElem] struct {
	Value     T
	Inclusive bool
	Infinite  bool
}

// Incl returns the finite inclusive bound v: the [ of a lower bound, the ] of
// an upper one.
func Incl[T RangeElem](v T) Bound[T] { return Bound[T]{Value: v, Inclusive: true} }

// Excl returns the finite exclusive bound v: the ( of a lower bound, the ) of an
// upper one.
func Excl[T RangeElem](v T) Bound[T] { return Bound[T]{Value: v} }

// NoBound returns the infinite bound: no lower limit as a lower bound, no upper
// limit as an upper one.
func NoBound[T RangeElem]() Bound[T] { return Bound[T]{Infinite: true} }

// Range is a PostgreSQL range of T, with empty, infinite and
// inclusive/exclusive bounds: int4range for int32, int8range for int64,
// numrange for Numeric, daterange for Date, tsrange for Timestamp and tstzrange
// for time.Time. A range column is a Range field, read and written through
// Value and Scan, which speak the text form PostgreSQL prints, such as
// [1,10), (,5] or empty.
//
// Empty is the empty range; Lower and Upper mean nothing then, and a range read
// back from the database as empty has them zero. PostgreSQL stores a discrete
// range (int4range, int8range, daterange) in the canonical form [lower,upper):
// a range written (0,9] reads back as [1,10), so compare ranges of those types
// in that form. The other types keep the bounds as written.
//
// Range types exist on PostgreSQL alone. The generator refuses a Range field
// when the target dialect is SQLite, MySQL or MariaDB, and the rangeop
// predicates fail the build with momo.ErrUnsupported there.
type Range[T RangeElem] struct {
	Lower, Upper Bound[T]
	Empty        bool
}

// NewRange returns the range from lower to upper.
func NewRange[T RangeElem](lower, upper Bound[T]) Range[T] {
	return Range[T]{Lower: lower, Upper: upper}
}

// ClosedOpen returns [lower, upper), the form PostgreSQL stores a discrete
// range in and the usual one for a period of time.
func ClosedOpen[T RangeElem](lower, upper T) Range[T] {
	return Range[T]{Lower: Incl(lower), Upper: Excl(upper)}
}

// EmptyRange returns the empty range.
func EmptyRange[T RangeElem]() Range[T] { return Range[T]{Empty: true} }

// IsEmpty reports whether r is the empty range.
func (r Range[T]) IsEmpty() bool { return r.Empty }

// rangeTypeName is the PostgreSQL range type of T.
func rangeTypeName[T RangeElem]() string {
	var z T
	switch any(z).(type) {
	case int32:
		return "int4range"
	case int64:
		return "int8range"
	case Numeric:
		return "numrange"
	case Date:
		return "daterange"
	case Timestamp:
		return "tsrange"
	}
	return "tstzrange"
}

// SQLType returns the column type for d: the range type on PostgreSQL, and the
// empty string on the other dialects, which have no range type.
func (Range[T]) SQLType(d dialect.Name) string {
	if d == dialect.Postgres {
		return rangeTypeName[T]()
	}
	return ""
}

// Value encodes r as PostgreSQL range text: "empty", or the bounds in
// brackets such as [1,10) with an infinite side left blank. An element that is
// not valid for its type (a Numeric with a flag and digits, a Date with both
// infinities) is an error.
func (r Range[T]) Value() (driver.Value, error) {
	if r.Empty {
		return "empty", nil
	}
	var sb strings.Builder
	if r.Lower.Infinite {
		sb.WriteByte('(')
	} else {
		if r.Lower.Inclusive {
			sb.WriteByte('[')
		} else {
			sb.WriteByte('(')
		}
		s, err := formatRangeElem(r.Lower.Value)
		if err != nil {
			return nil, err
		}
		sb.WriteString(quoteRangeElem(s))
	}
	sb.WriteByte(',')
	if r.Upper.Infinite {
		sb.WriteByte(')')
		return sb.String(), nil
	}
	s, err := formatRangeElem(r.Upper.Value)
	if err != nil {
		return nil, err
	}
	sb.WriteString(quoteRangeElem(s))
	if r.Upper.Inclusive {
		sb.WriteByte(']')
	} else {
		sb.WriteByte(')')
	}
	return sb.String(), nil
}

// String returns the text form Value writes, or the error text when an element
// is not valid.
func (r Range[T]) String() string {
	v, err := r.Value()
	if err != nil {
		return err.Error()
	}
	return v.(string)
}

// Scan accepts the PostgreSQL range text as string or []byte: empty, or
// bounds in brackets with an infinite side left blank. The database driver
// delivers range columns in this form whatever the wire format. Scan(nil)
// returns an error: use a *Range for a nullable column.
func (r *Range[T]) Scan(src any) error {
	s, err := text("Range", src)
	if err != nil {
		return err
	}
	v, err := parseRange[T](s)
	if err != nil {
		return err
	}
	*r = v
	return nil
}

// formatRangeElem writes v the way PostgreSQL reads it inside a range.
func formatRangeElem[T RangeElem](v T) (string, error) {
	switch x := any(v).(type) {
	case int32:
		return strconv.FormatInt(int64(x), 10), nil
	case int64:
		return strconv.FormatInt(x, 10), nil
	case Numeric:
		return valuerText(x)
	case Date:
		return valuerText(x)
	case Timestamp:
		return valuerText(x)
	case time.Time:
		return x.UTC().Format(zoneMinute), nil
	}
	return "", fmt.Errorf("types: %T is not a range element", v)
}

// valuerText is the text a Valuer encodes to.
func valuerText(v driver.Valuer) (string, error) {
	out, err := v.Value()
	if err != nil {
		return "", err
	}
	s, ok := out.(string)
	if !ok {
		return "", fmt.Errorf("types: %T encodes as %T, not text", v, out)
	}
	return s, nil
}

// quoteRangeElem double-quotes s when range text would otherwise misread it: a
// blank, or a comma, bracket, parenthesis, quote, backslash or space. A quote
// and a backslash inside are escaped with a backslash.
func quoteRangeElem(s string) string {
	if s != "" && !strings.ContainsAny(s, " ,()[]\"\\") {
		return s
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// parseRange reads PostgreSQL range text into a Range of T.
func parseRange[T RangeElem](s string) (Range[T], error) {
	s = strings.TrimSpace(s)
	if strings.EqualFold(s, "empty") {
		return Range[T]{Empty: true}, nil
	}
	bad := func(why string) (Range[T], error) {
		return Range[T]{}, fmt.Errorf("types: invalid Range %q: %s", s, why)
	}
	if len(s) < 3 {
		return bad("too short")
	}
	open, closing := s[0], s[len(s)-1]
	if (open != '[' && open != '(') || (closing != ']' && closing != ')') {
		return bad("want [ or ( first and ] or ) last")
	}
	lower, upper, err := splitRangeBounds(s[1 : len(s)-1])
	if err != nil {
		return bad(err.Error())
	}
	var r Range[T]
	if lower.present {
		v, err := parseRangeElem[T](lower.text)
		if err != nil {
			return bad(err.Error())
		}
		r.Lower = Bound[T]{Value: v, Inclusive: open == '['}
	} else {
		r.Lower = NoBound[T]()
	}
	if upper.present {
		v, err := parseRangeElem[T](upper.text)
		if err != nil {
			return bad(err.Error())
		}
		r.Upper = Bound[T]{Value: v, Inclusive: closing == ']'}
	} else {
		r.Upper = NoBound[T]()
	}
	return r, nil
}

// rangeToken is one side of the comma of range text. present is false for a
// blank unquoted side, which is an infinite bound; a quoted empty string is a
// present element with no text.
type rangeToken struct {
	text    string
	present bool
}

// splitRangeBounds splits the text between the brackets at its one top-level
// comma. A side may be double-quoted, with "" or \" for a quote and \\ for a
// backslash inside, and a backslash outside quotes escapes the next character.
func splitRangeBounds(s string) (lower, upper rangeToken, err error) {
	var toks []rangeToken
	var cur strings.Builder
	quoted, inQuote := false, false
	flush := func() {
		toks = append(toks, rangeToken{text: cur.String(), present: quoted || cur.Len() > 0})
		cur.Reset()
		quoted = false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\':
			i++
			if i >= len(s) {
				return lower, upper, errors.New("a backslash ends the text")
			}
			cur.WriteByte(s[i])
		case c == '"' && inQuote:
			if i+1 < len(s) && s[i+1] == '"' {
				cur.WriteByte('"')
				i++
			} else {
				inQuote = false
			}
		case c == '"':
			inQuote, quoted = true, true
		case c == ',' && !inQuote:
			flush()
		default:
			cur.WriteByte(c)
		}
	}
	if inQuote {
		return lower, upper, errors.New("an unterminated quote")
	}
	flush()
	if len(toks) != 2 {
		return lower, upper, fmt.Errorf("want two bounds separated by a comma, got %d", len(toks))
	}
	return toks[0], toks[1], nil
}

// parseRangeElem reads one bound of text into a T.
func parseRangeElem[T RangeElem](s string) (T, error) {
	var zero T
	var out any
	switch any(zero).(type) {
	case int32:
		n, err := strconv.ParseInt(s, 10, 32)
		if err != nil {
			return zero, fmt.Errorf("bound %q is not an int4: %w", s, err)
		}
		out = int32(n)
	case int64:
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return zero, fmt.Errorf("bound %q is not an int8: %w", s, err)
		}
		out = n
	case Numeric:
		n, err := ParseNumeric(s)
		if err != nil {
			return zero, err
		}
		out = n
	case Date:
		var d Date
		if err := d.Scan(s); err != nil {
			return zero, err
		}
		out = d
	case Timestamp:
		var t Timestamp
		if err := t.Scan(s); err != nil {
			return zero, err
		}
		out = t
	case time.Time:
		var t time.Time
		var err error
		for _, layout := range []string{zoneHour, zoneMinute, zoneSecond, time.RFC3339Nano} {
			if t, err = time.Parse(layout, s); err == nil {
				break
			}
		}
		if err != nil {
			return zero, fmt.Errorf("bound %q is not a timestamp with a time zone", s)
		}
		out = t.UTC()
	}
	v, ok := out.(T)
	if !ok {
		return zero, fmt.Errorf("%T is not a range element", zero)
	}
	return v, nil
}
