// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/azrtydxb/momo/dialect"
)

// Typed date and time expressions: Extract, AtTimeZone, intervals with
// AddInterval and SubInterval, Now, CurrentTimestamp and DateTrunc. Every
// keyword that reaches the SQL text (an extract field, an interval unit, a
// truncation unit) is looked up in a fixed allowlist at build time and the
// canonical spelling from the table is what gets written, so no caller
// string is ever interpolated. A name outside the allowlist fails the build
// with ErrValidation.

func timeErr(format string, a ...any) *Error {
	return &Error{Kind: ErrValidation, Op: "build", Err: fmt.Errorf(format, a...)}
}

// sqliteTimeFmt is the timestamp text format momo's SQLite schema uses
// (strftime('%Y-%m-%d %H:%M:%f', ...)); expressions that return a timestamp
// on SQLite produce it so they compare and sort with stored values.
const sqliteTimeFmt = `'%Y-%m-%d %H:%M:%f'`

// maxIntervalMagnitude bounds a literal interval count, so scaling it by a
// unit factor can never overflow an int64.
const maxIntervalMagnitude = 1_000_000_000_000

// appendTime renders a timestamp operand. On PostgreSQL a bound Go time
// (Value(t)) is cast to timestamptz, because the server cannot always infer
// the parameter type inside EXTRACT, date_trunc or AT TIME ZONE.
func appendTime(b *Builder, e Expression[time.Time]) {
	if t, ok := e.(typed[time.Time]); ok && b.d.Name == dialect.Postgres {
		if _, bound := t.Expr.(argExpr); bound {
			b.WriteString("CAST(")
			e.AppendSQL(b)
			b.WriteString(" AS timestamptz)")
			return
		}
	}
	e.AppendSQL(b)
}

// ------------------------------------------------------------- Extract

// ExtractField names the part of a timestamp that Extract reads. Use the
// constants; any other value fails the build with ErrValidation.
type ExtractField string

// The extract fields every dialect can answer. Dow counts Sunday as 0 and
// ISODow counts Monday as 1 through Sunday as 7.
const (
	ExtractYear    ExtractField = "year"
	ExtractMonth   ExtractField = "month"
	ExtractDay     ExtractField = "day"
	ExtractHour    ExtractField = "hour"
	ExtractMinute  ExtractField = "minute"
	ExtractSecond  ExtractField = "second"
	ExtractQuarter ExtractField = "quarter"
	ExtractDow     ExtractField = "dow"
	ExtractISODow  ExtractField = "isodow"
	ExtractDoy     ExtractField = "doy"
	ExtractEpoch   ExtractField = "epoch"
)

// extractPG maps the allowlisted fields to their PostgreSQL keywords.
var extractPG = map[ExtractField]string{
	ExtractYear: "YEAR", ExtractMonth: "MONTH", ExtractDay: "DAY",
	ExtractHour: "HOUR", ExtractMinute: "MINUTE", ExtractSecond: "SECOND",
	ExtractQuarter: "QUARTER", ExtractDow: "DOW", ExtractISODow: "ISODOW",
	ExtractDoy: "DOY", ExtractEpoch: "EPOCH",
}

// ExtractExpr is a date part of a timestamp as a float64. Build it with
// Extract.
type ExtractExpr struct {
	exprOps[float64]
	field ExtractField
	e     Expression[time.Time]
}

// Extract reads one part of a timestamp: EXTRACT(field FROM e) on
// PostgreSQL, strftime on SQLite and EXTRACT, DAYOFWEEK and friends on
// MySQL. The result is always a float64 (cast on PostgreSQL, whose EXTRACT
// returns numeric), so ExtractEpoch can carry fractional seconds; wrap it in
// Cast to get an integer. On PostgreSQL ExtractSecond includes the
// fraction; SQLite and MySQL return whole seconds. On MySQL ExtractEpoch is
// UNIX_TIMESTAMP, which reads the value in the session time zone.
func Extract(field ExtractField, e Expression[time.Time]) ExtractExpr {
	x := ExtractExpr{field: field, e: e}
	x.exprOps = selfOps[float64](x)
	return x
}

// As names the expression in a select list.
func (x ExtractExpr) As(alias string) Aliased[float64] { return As[float64](x, alias) }

func (ExtractExpr) resultType() float64 { return 0 }

func (x ExtractExpr) AppendSQL(b *Builder) {
	kw, ok := extractPG[x.field]
	if !ok {
		b.SetErr(timeErr("extract field %q is not one of the Extract... constants", string(x.field)))
		return
	}
	if x.e == nil {
		b.SetErr(timeErr("Extract needs a timestamp expression"))
		return
	}
	switch b.d.Name {
	case dialect.Postgres:
		b.WriteString("CAST(EXTRACT(" + kw + " FROM ")
		appendTime(b, x.e)
		b.WriteString(") AS double precision)")
	case dialect.SQLite:
		x.appendSQLite(b)
	case dialect.MySQL:
		x.appendMySQL(b)
	default:
		b.unsupported("Extract")
	}
}

func (x ExtractExpr) appendSQLite(b *Builder) {
	strf := func(f string) {
		b.WriteString("strftime('" + f + "', ")
		x.e.AppendSQL(b)
		b.writeByte(')')
	}
	switch x.field {
	case ExtractQuarter:
		b.WriteString("CAST((CAST(")
		strf("%m")
		b.WriteString(" AS INTEGER) + 2) / 3 AS REAL)")
	case ExtractISODow:
		b.WriteString("CAST((CAST(")
		strf("%w")
		b.WriteString(" AS INTEGER) + 6) % 7 + 1 AS REAL)")
	default:
		f := map[ExtractField]string{
			ExtractYear: "%Y", ExtractMonth: "%m", ExtractDay: "%d",
			ExtractHour: "%H", ExtractMinute: "%M", ExtractSecond: "%S",
			ExtractDow: "%w", ExtractDoy: "%j", ExtractEpoch: "%s",
		}[x.field]
		b.WriteString("CAST(")
		strf(f)
		b.WriteString(" AS REAL)")
	}
}

func (x ExtractExpr) appendMySQL(b *Builder) {
	// "+ 0e0" turns MySQL's integer or decimal result into a DOUBLE.
	wrap := func(pre string, post string) {
		b.WriteString("(" + pre)
		x.e.AppendSQL(b)
		b.WriteString(post + " + 0e0)")
	}
	switch x.field {
	case ExtractDow:
		wrap("DAYOFWEEK(", ") - 1")
	case ExtractISODow:
		wrap("WEEKDAY(", ") + 1")
	case ExtractDoy:
		wrap("DAYOFYEAR(", ")")
	case ExtractEpoch:
		wrap("UNIX_TIMESTAMP(", ")")
	default:
		wrap("EXTRACT("+extractPG[x.field]+" FROM ", ")")
	}
}

// ------------------------------------------------------------ TimeExpr

// TimeExpr is a timestamp-valued expression built by AtTimeZone,
// AddInterval, SubInterval, Now, CurrentTimestamp or DateTrunc. It carries
// the comparisons of a column (Eq, Lt, Between, ...) and can be aliased with
// As.
type TimeExpr struct {
	exprOps[time.Time]
	fn func(*Builder)
}

func newTimeExpr(fn func(*Builder)) TimeExpr {
	t := TimeExpr{fn: fn}
	t.exprOps = selfOps[time.Time](t)
	return t
}

// As names the expression in a select list.
func (t TimeExpr) As(alias string) Aliased[time.Time] { return As[time.Time](t, alias) }

func (TimeExpr) resultType() time.Time { return time.Time{} }

func (t TimeExpr) AppendSQL(b *Builder) {
	if t.fn == nil {
		b.SetErr(timeErr("expression was not built with its constructor"))
		return
	}
	t.fn(b)
}

// ---------------------------------------------------------- AtTimeZone

// AtTimeZone is (e AT TIME ZONE 'zone') on PostgreSQL: a timestamptz becomes
// the wall-clock timestamp without time zone in zone, and a timestamp is read
// as wall-clock in zone and becomes a timestamptz. zone is an IANA name such
// as Europe/Brussels. It is written as a quoted literal, not bound, so the
// same expression in SELECT and GROUP BY renders identical text and
// PostgreSQL accepts the grouping; for that it must be at most 64 bytes of
// letters, digits and _ + : / -, otherwise the build fails with
// ErrValidation. Use AtTimeZoneExpr for a zone held in a column. SQLite and
// MySQL have no portable time zone conversion and fail with ErrUnsupported.
//
// For local-time buckets wrap the column before truncating or extracting:
// DateTrunc(TruncDay, AtTimeZone(col, "Europe/Brussels")). DateTrunc and
// Extract on a bare timestamptz work in the session time zone.
func AtTimeZone(e Expression[time.Time], zone string) TimeExpr {
	if err := validZone(zone); err != nil {
		return newTimeExpr(func(b *Builder) { b.SetErr(timeErr("AtTimeZone zone %q: %v", zone, err)) })
	}
	return AtTimeZoneExpr(e, zoneLiteral(zone))
}

// zoneLiteral is a validated zone name rendered as a quoted SQL literal.
type zoneLiteral string

func (zoneLiteral) resultType() string { return "" }

func (z zoneLiteral) AppendSQL(b *Builder) { b.WriteString("'" + string(z) + "'") }

func validZone(zone string) error {
	if zone == "" || len(zone) > 64 {
		return errors.New("must be 1 to 64 bytes")
	}
	for i := 0; i < len(zone); i++ {
		c := zone[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			c == '_', c == '+', c == ':', c == '/', c == '-':
		default:
			return fmt.Errorf("invalid byte %q", c)
		}
	}
	return nil
}

// AtTimeZoneExpr is AtTimeZone with the zone taken from a string
// expression, for example a column holding each row's zone.
func AtTimeZoneExpr(e Expression[time.Time], zone Expression[string]) TimeExpr {
	return newTimeExpr(func(b *Builder) {
		if e == nil || zone == nil {
			b.SetErr(timeErr("AtTimeZone needs a timestamp and a zone expression"))
			return
		}
		if b.d.Name != dialect.Postgres {
			b.unsupported("AtTimeZone")
			return
		}
		b.writeByte('(')
		appendTime(b, e)
		b.WriteString(" AT TIME ZONE ")
		zone.AppendSQL(b)
		b.writeByte(')')
	})
}

// ------------------------------------------------------------ Interval

// IntervalUnit is the unit of an Interval. Use the constants; any other
// value fails the build with ErrValidation.
type IntervalUnit string

// The interval units every dialect supports.
const (
	IntervalSecond IntervalUnit = "second"
	IntervalMinute IntervalUnit = "minute"
	IntervalHour   IntervalUnit = "hour"
	IntervalDay    IntervalUnit = "day"
	IntervalWeek   IntervalUnit = "week"
	IntervalMonth  IntervalUnit = "month"
	IntervalYear   IntervalUnit = "year"
)

type unitSpec struct {
	pg     string // PostgreSQL INTERVAL unit word
	my     string // MySQL INTERVAL unit keyword
	sqlite string // SQLite date modifier unit
	factor int64  // multiplier into the SQLite unit
}

var intervalUnits = map[IntervalUnit]unitSpec{
	IntervalSecond: {"second", "SECOND", "seconds", 1},
	IntervalMinute: {"minute", "MINUTE", "minutes", 1},
	IntervalHour:   {"hour", "HOUR", "hours", 1},
	IntervalDay:    {"day", "DAY", "days", 1},
	IntervalWeek:   {"week", "WEEK", "days", 7},
	IntervalMonth:  {"month", "MONTH", "months", 1},
	IntervalYear:   {"year", "YEAR", "years", 1},
}

// Interval is a length of time: a count and a unit. It is not an expression
// of its own, because MySQL only accepts an interval as an operand of date
// arithmetic; combine it with a timestamp through AddInterval or
// SubInterval. Build it with IntervalOf or IntervalExpr.
type Interval struct {
	lit     int64
	count   Expression[int64] // nil for a literal
	unit    IntervalUnit
	isBuilt bool
	isExpr  bool
}

// IntervalOf is a literal interval of n units, written into the SQL text
// (INTERVAL '5 day'), so the same expression in SELECT and GROUP BY stays
// identical. n must be within one trillion either way, otherwise the build
// fails with ErrValidation.
func IntervalOf(n int64, unit IntervalUnit) Interval {
	return Interval{lit: n, unit: unit, isBuilt: true}
}

// IntervalExpr is an interval whose count comes from an integer expression:
// a column, or Value(n) to bind a parameter. On PostgreSQL the count is
// multiplied into a one-unit INTERVAL.
func IntervalExpr(count Expression[int64], unit IntervalUnit) Interval {
	return Interval{count: count, unit: unit, isBuilt: true, isExpr: true}
}

// AddInterval is ts + iv: the timestamp moved forward by the interval.
// PostgreSQL renders (ts + INTERVAL ...), MySQL DATE_ADD and SQLite
// strftime with a modifier. The SQLite result is text in momo's timestamp
// format, in UTC, with millisecond precision. Month and year arithmetic
// differs at month ends: PostgreSQL and MySQL clamp to the last day (Jan 31
// plus one month is Feb 28 or 29), SQLite normalises the overflow into the
// next month (Mar 2 or 3). SQLite years outside 0000 to 9999 are NULL.
func AddInterval(ts Expression[time.Time], iv Interval) TimeExpr { return intervalExpr(ts, iv, 1) }

// SubInterval is ts - iv: the timestamp moved back by the interval.
func SubInterval(ts Expression[time.Time], iv Interval) TimeExpr { return intervalExpr(ts, iv, -1) }

func intervalExpr(ts Expression[time.Time], iv Interval, sign int64) TimeExpr {
	return newTimeExpr(func(b *Builder) {
		spec, ok := intervalUnits[iv.unit]
		switch {
		case ts == nil || !iv.isBuilt:
			b.SetErr(timeErr("interval arithmetic needs a timestamp and an interval built with IntervalOf or IntervalExpr"))
			return
		case iv.isExpr && iv.count == nil:
			b.SetErr(timeErr("IntervalExpr needs a count expression"))
			return
		case !ok:
			b.SetErr(timeErr("interval unit %q is not one of the Interval... constants", string(iv.unit)))
			return
		case !iv.isExpr && (iv.lit > maxIntervalMagnitude || iv.lit < -maxIntervalMagnitude):
			b.SetErr(timeErr("interval of %d %ss is outside the supported range", iv.lit, string(iv.unit)))
			return
		}
		switch b.d.Name {
		case dialect.Postgres:
			b.writeByte('(')
			appendTime(b, ts)
			op := " + "
			if sign < 0 {
				op = " - "
			}
			b.WriteString(op)
			if !iv.isExpr {
				b.WriteString("INTERVAL '" + strconv.FormatInt(iv.lit, 10) + " " + spec.pg + "')")
				return
			}
			b.WriteString("(CAST(")
			iv.count.AppendSQL(b)
			b.WriteString(" AS bigint) * INTERVAL '1 " + spec.pg + "'))")
		case dialect.MySQL:
			fn := "DATE_ADD("
			if sign < 0 {
				fn = "DATE_SUB("
			}
			b.WriteString(fn)
			ts.AppendSQL(b)
			b.WriteString(", INTERVAL ")
			if !iv.isExpr {
				b.WriteString(strconv.FormatInt(iv.lit, 10))
			} else {
				iv.count.AppendSQL(b)
			}
			b.WriteString(" " + spec.my + ")")
		case dialect.SQLite:
			b.WriteString("strftime(" + sqliteTimeFmt + ", ")
			ts.AppendSQL(b)
			b.WriteString(", ")
			if !iv.isExpr {
				b.WriteString("'" + strconv.FormatInt(iv.lit*sign*spec.factor, 10) + " " + spec.sqlite + "')")
				return
			}
			b.WriteString("CAST((")
			iv.count.AppendSQL(b)
			b.WriteString(") * " + strconv.FormatInt(sign*spec.factor, 10) + " AS TEXT) || ' " + spec.sqlite + "')")
		default:
			b.unsupported("interval arithmetic")
		}
	})
}

// ----------------------------------------------------------------- Now

func nowExpr(name string, pg, my string) TimeExpr {
	return newTimeExpr(func(b *Builder) {
		switch b.d.Name {
		case dialect.Postgres:
			b.WriteString(pg)
		case dialect.MySQL:
			b.WriteString(my)
		case dialect.SQLite:
			b.WriteString("strftime(" + sqliteTimeFmt + ", 'now')")
		default:
			b.unsupported(name)
		}
	})
}

// Now is the database clock: now() on PostgreSQL (the start of the
// transaction), NOW(6) on MySQL, and strftime of 'now' in momo's timestamp
// format on SQLite. It reads the database's time, not the WithClock option.
func Now() TimeExpr { return nowExpr("Now", "now()", "NOW(6)") }

// CurrentTimestamp is CURRENT_TIMESTAMP, with microsecond precision on
// MySQL, and the same millisecond text as Now on SQLite.
func CurrentTimestamp() TimeExpr {
	return nowExpr("CurrentTimestamp", "CURRENT_TIMESTAMP", "CURRENT_TIMESTAMP(6)")
}

// ----------------------------------------------------------- DateTrunc

// TruncUnit is the precision of DateTrunc. Use the constants; any other
// value fails the build with ErrValidation.
type TruncUnit string

// The truncation units every dialect supports. Weeks start on Monday.
const (
	TruncSecond  TruncUnit = "second"
	TruncMinute  TruncUnit = "minute"
	TruncHour    TruncUnit = "hour"
	TruncDay     TruncUnit = "day"
	TruncWeek    TruncUnit = "week"
	TruncMonth   TruncUnit = "month"
	TruncQuarter TruncUnit = "quarter"
	TruncYear    TruncUnit = "year"
)

// truncFormats maps the allowlisted units to the strftime or DATE_FORMAT
// layout that keeps everything above the unit and zeroes the rest. The
// layouts contain no quote, so they are safe inside a SQL string.
var truncFormats = map[TruncUnit]struct{ sqlite, mysql string }{
	TruncSecond:  {"%Y-%m-%d %H:%M:%S.000", "%Y-%m-%d %H:%i:%s"},
	TruncMinute:  {"%Y-%m-%d %H:%M:00.000", "%Y-%m-%d %H:%i:00"},
	TruncHour:    {"%Y-%m-%d %H:00:00.000", "%Y-%m-%d %H:00:00"},
	TruncDay:     {"%Y-%m-%d 00:00:00.000", "%Y-%m-%d"},
	TruncWeek:    {"%Y-%m-%d 00:00:00.000", "%Y-%m-%d"},
	TruncMonth:   {"%Y-%m-01 00:00:00.000", "%Y-%m-01"},
	TruncQuarter: {"", ""},
	TruncYear:    {"%Y-01-01 00:00:00.000", "%Y-01-01"},
}

// DateTrunc rounds a timestamp down to the start of its unit:
// date_trunc('day', e) on PostgreSQL, and the equivalent strftime or
// DATE_FORMAT composition on SQLite and MySQL. The unit is written as a
// literal so SELECT and GROUP BY render identically. On PostgreSQL a
// timestamptz is truncated in the session time zone; wrap the input in
// AtTimeZone for buckets in a chosen zone. On SQLite the result
// is text in momo's timestamp format; on MySQL it is a DATETIME.
func DateTrunc(unit TruncUnit, e Expression[time.Time]) TimeExpr {
	return newTimeExpr(func(b *Builder) {
		f, ok := truncFormats[unit]
		switch {
		case !ok:
			b.SetErr(timeErr("truncation unit %q is not one of the Trunc... constants", string(unit)))
			return
		case e == nil:
			b.SetErr(timeErr("DateTrunc needs a timestamp expression"))
			return
		}
		switch b.d.Name {
		case dialect.Postgres:
			b.WriteString("date_trunc('" + string(unit) + "', ")
			appendTime(b, e)
			b.writeByte(')')
		case dialect.SQLite:
			truncSQLite(b, unit, f.sqlite, e)
		case dialect.MySQL:
			truncMySQL(b, unit, f.mysql, e)
		default:
			b.unsupported("DateTrunc")
		}
	})
}

func truncSQLite(b *Builder, unit TruncUnit, layout string, e Expr) {
	switch unit {
	case TruncQuarter:
		b.WriteString("(strftime('%Y-', ")
		e.AppendSQL(b)
		b.WriteString(") || printf('%02d', (CAST(strftime('%m', ")
		e.AppendSQL(b)
		b.WriteString(") AS INTEGER) - 1) / 3 * 3 + 1) || '-01 00:00:00.000')")
	case TruncWeek:
		b.WriteString("strftime('" + layout + "', ")
		e.AppendSQL(b)
		b.WriteString(", 'start of day', '+1 day', 'weekday 1', '-7 days')")
	default:
		b.WriteString("strftime('" + layout + "', ")
		e.AppendSQL(b)
		b.writeByte(')')
	}
}

func truncMySQL(b *Builder, unit TruncUnit, layout string, e Expr) {
	switch unit {
	case TruncQuarter:
		b.WriteString("CAST(CONCAT(YEAR(")
		e.AppendSQL(b)
		b.WriteString("), '-', (QUARTER(")
		e.AppendSQL(b)
		b.WriteString(") - 1) * 3 + 1, '-01') AS DATETIME)")
	case TruncWeek:
		b.WriteString("CAST(DATE_FORMAT(DATE_SUB(")
		e.AppendSQL(b)
		b.WriteString(", INTERVAL WEEKDAY(")
		e.AppendSQL(b)
		b.WriteString(") DAY), '" + layout + "') AS DATETIME)")
	default:
		b.WriteString("CAST(DATE_FORMAT(")
		e.AppendSQL(b)
		b.WriteString(", '" + layout + "') AS DATETIME)")
	}
}
