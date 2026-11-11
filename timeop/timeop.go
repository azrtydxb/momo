// SPDX-License-Identifier: Apache-2.0

// Package timeop holds the typed date and time expressions: Extract,
// AtTimeZone, intervals with AddInterval, SubInterval, AddDuration and
// SubDuration (down to the microsecond), timestamp differences with Sub, Age
// and EpochSeconds, Now,
// CurrentTimestamp, ClockTimestamp, StatementTimestamp and Trunc. Every
// keyword that reaches the SQL text (an extract field, an interval unit, a truncation unit) is looked up in a fixed
// allowlist at build time and the canonical spelling from the table is what
// gets written, so no caller string is ever interpolated. A name outside the
// allowlist fails the build with momo.ErrValidation.
//
// The package builds on the exported extension API of the root package
// (Ops, NewOps, Typed, As, IsBound and the Builder methods) and uses no
// unexported name of it.
package timeop

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

func timeErr(format string, a ...any) *momo.Error {
	return &momo.Error{Kind: momo.ErrValidation, Op: "build", Err: fmt.Errorf(format, a...)}
}

// sqliteTimeFmt is the timestamp text format momo's SQLite schema uses
// (strftime('%Y-%m-%d %H:%M:%f', ...)); expressions that return a timestamp
// on SQLite produce it so they compare and sort with stored values.
const sqliteTimeFmt = `'%Y-%m-%d %H:%M:%f'`

// maxIntervalMagnitude bounds a literal interval count, so scaling it by a
// unit factor can never overflow an int64.
const maxIntervalMagnitude = 1_000_000_000_000

// Sub-second interval counts scale by 1000 on MySQL, so their bounds are
// higher (a time.Duration reaches about 9.2e15 microseconds) and still cannot
// overflow an int64 when scaled.
const (
	maxMilliMagnitude = 1_000_000_000_000_000
	maxMicroMagnitude = 10_000_000_000_000_000
)

// appendTime renders a timestamp operand. On PostgreSQL a bound Go time
// (momo.Value(t)) is cast to timestamptz, because the server cannot always infer
// the parameter type inside EXTRACT, date_trunc or AT TIME ZONE.
func appendTime(b *momo.Builder, e momo.Expression[time.Time]) {
	if b.Dialect().Name == dialect.Postgres && momo.IsBound(e) {
		b.WriteString("CAST(")
		e.AppendSQL(b)
		b.WriteString(" AS timestamptz)")
		return
	}
	e.AppendSQL(b)
}

// ------------------------------------------------------------- Extract

// ExtractField names the part of a timestamp that Extract reads. Use the
// constants; any other value fails the build with momo.ErrValidation.
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
	momo.Ops[float64]
	field ExtractField
	e     momo.Expression[time.Time]
}

// Extract reads one part of a timestamp: EXTRACT(field FROM e) on
// PostgreSQL, strftime on SQLite and EXTRACT, DAYOFWEEK and friends on
// MySQL. The result is always a float64 (cast on PostgreSQL, whose EXTRACT
// returns numeric), so ExtractEpoch can carry fractional seconds; wrap it in
// sqlfn.Cast to get an integer. On PostgreSQL ExtractSecond includes the
// fraction; SQLite and MySQL return whole seconds. On MySQL ExtractEpoch is
// UNIX_TIMESTAMP, which reads the value in the session time zone.
func Extract(field ExtractField, e momo.Expression[time.Time]) ExtractExpr {
	x := ExtractExpr{field: field, e: e}
	x.Ops = momo.NewOps[float64](x)
	return x
}

// As names the expression in a select list.
func (x ExtractExpr) As(alias string) momo.Aliased[float64] { return momo.As[float64](x, alias) }

// AppendSQL renders the extraction for the dialect of b.
func (x ExtractExpr) AppendSQL(b *momo.Builder) {
	kw, ok := extractPG[x.field]
	if !ok {
		b.SetErr(timeErr("extract field %q is not one of the Extract... constants", string(x.field)))
		return
	}
	if x.e == nil {
		b.SetErr(timeErr("Extract needs a timestamp expression"))
		return
	}
	switch b.Dialect().Name {
	case dialect.Postgres:
		b.WriteString("CAST(EXTRACT(" + kw + " FROM ")
		appendTime(b, x.e)
		b.WriteString(") AS double precision)")
	case dialect.SQLite:
		x.appendSQLite(b)
	case dialect.MySQL:
		x.appendMySQL(b)
	default:
		b.Unsupported("Extract")
	}
}

func (x ExtractExpr) appendSQLite(b *momo.Builder) {
	strf := func(f string) {
		b.WriteString("strftime('" + f + "', ")
		x.e.AppendSQL(b)
		b.WriteString(")")
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

func (x ExtractExpr) appendMySQL(b *momo.Builder) {
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

// ------------------------------------------------------------------ Expr

// Expr is a timestamp-valued expression built by AtTimeZone, AddInterval,
// SubInterval, Now, CurrentTimestamp or Trunc. It carries the comparisons of
// a column (Eq, Lt, Between, ...) and can be aliased with As.
type Expr struct {
	momo.Ops[time.Time]
	fn func(*momo.Builder)
}

func newExpr(fn func(*momo.Builder)) Expr {
	t := Expr{fn: fn}
	t.Ops = momo.NewOps[time.Time](t)
	return t
}

// As names the expression in a select list.
func (t Expr) As(alias string) momo.Aliased[time.Time] { return momo.As[time.Time](t, alias) }

// AppendSQL renders the expression for the dialect of b.
func (t Expr) AppendSQL(b *momo.Builder) {
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
// momo.ErrValidation. Use AtTimeZoneExpr for a zone held in a column. SQLite and
// MySQL have no portable time zone conversion and fail with momo.ErrUnsupported.
//
// For local-time buckets wrap the column before truncating or extracting:
// Trunc(TruncDay, AtTimeZone(col, "Europe/Brussels")). Trunc and
// Extract on a bare timestamptz work in the session time zone.
func AtTimeZone(e momo.Expression[time.Time], zone string) Expr {
	if err := validZone(zone); err != nil {
		return newExpr(func(b *momo.Builder) { b.SetErr(timeErr("AtTimeZone zone %q: %v", zone, err)) })
	}
	return AtTimeZoneExpr(e, momo.Typed[string](zoneLiteral(zone)))
}

// zoneLiteral is a validated zone name rendered as a quoted SQL literal.
type zoneLiteral string

// AppendSQL writes the validated zone as a quoted literal.
func (z zoneLiteral) AppendSQL(b *momo.Builder) { b.WriteString("'" + string(z) + "'") }

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
func AtTimeZoneExpr(e momo.Expression[time.Time], zone momo.Expression[string]) Expr {
	return newExpr(func(b *momo.Builder) {
		if e == nil || zone == nil {
			b.SetErr(timeErr("AtTimeZone needs a timestamp and a zone expression"))
			return
		}
		if b.Dialect().Name != dialect.Postgres {
			b.Unsupported("AtTimeZone")
			return
		}
		b.WriteString("(")
		appendTime(b, e)
		b.WriteString(" AT TIME ZONE ")
		zone.AppendSQL(b)
		b.WriteString(")")
	})
}

// ------------------------------------------------------------ Interval

// IntervalUnit is the unit of an Interval. Use the constants; any other
// value fails the build with momo.ErrValidation.
type IntervalUnit string

// The interval units every dialect supports.
const (
	IntervalMicrosecond IntervalUnit = "microsecond"
	IntervalMillisecond IntervalUnit = "millisecond"
	IntervalSecond      IntervalUnit = "second"
	IntervalMinute      IntervalUnit = "minute"
	IntervalHour        IntervalUnit = "hour"
	IntervalDay         IntervalUnit = "day"
	IntervalWeek        IntervalUnit = "week"
	IntervalMonth       IntervalUnit = "month"
	IntervalYear        IntervalUnit = "year"
)

type unitSpec struct {
	pg       string // PostgreSQL INTERVAL unit word
	my       string // MySQL INTERVAL unit keyword
	myFactor int64  // multiplier into the MySQL unit
	sqlite   string // SQLite date modifier unit
	factor   int64  // multiplier into the SQLite unit
	frac     int    // SQLite: decimal digits of seconds per count (3 for milliseconds); 0 for whole units
	maxMag   int64  // largest literal count in either direction
}

var intervalUnits = map[IntervalUnit]unitSpec{
	IntervalMicrosecond: {"microsecond", "MICROSECOND", 1, "seconds", 1, 6, maxMicroMagnitude},
	IntervalMillisecond: {"millisecond", "MICROSECOND", 1000, "seconds", 1, 3, maxMilliMagnitude},
	IntervalSecond:      {"second", "SECOND", 1, "seconds", 1, 0, maxIntervalMagnitude},
	IntervalMinute:      {"minute", "MINUTE", 1, "minutes", 1, 0, maxIntervalMagnitude},
	IntervalHour:        {"hour", "HOUR", 1, "hours", 1, 0, maxIntervalMagnitude},
	IntervalDay:         {"day", "DAY", 1, "days", 1, 0, maxIntervalMagnitude},
	IntervalWeek:        {"week", "WEEK", 1, "days", 7, 0, maxIntervalMagnitude},
	IntervalMonth:       {"month", "MONTH", 1, "months", 1, 0, maxIntervalMagnitude},
	IntervalYear:        {"year", "YEAR", 1, "years", 1, 0, maxIntervalMagnitude},
}

// Interval is a length of time: a count and a unit. It is not an expression
// of its own, because MySQL only accepts an interval as an operand of date
// arithmetic; combine it with a timestamp through AddInterval or
// SubInterval. Build it with IntervalOf or IntervalExpr.
type Interval struct {
	lit     int64
	count   momo.Expression[int64] // nil for a literal
	unit    IntervalUnit
	isBuilt bool
	isExpr  bool
}

// IntervalOf is a literal interval of n units, written into the SQL text
// (INTERVAL '5 day'), so the same expression in SELECT and GROUP BY stays
// identical. n must be within one trillion either way, otherwise the build
// fails with momo.ErrValidation.
func IntervalOf(n int64, unit IntervalUnit) Interval {
	return Interval{lit: n, unit: unit, isBuilt: true}
}

// IntervalDuration is a literal interval of d, in the coarsest of second,
// millisecond and microsecond units that holds d exactly; a d finer than a
// microsecond is truncated toward zero to whole microseconds. It is the
// time.Duration form of IntervalOf, for a TTL or lease length held as a
// duration: 1500ms is 1500 milliseconds, not a rounded 2 seconds. SQLite
// timestamps carry millisecond precision: it rounds a sub-millisecond part to
// the nearest millisecond, half away from zero (1500us adds 2 ms, 1499us adds
// 1 ms). Each distinct duration renders a distinct SQL literal, so a caller
// with many different durations gets many distinct statement texts, which
// matters for statement caching and pg_stat_statements; bind the count with
// IntervalExpr to keep one text.
func IntervalDuration(d time.Duration) Interval {
	switch {
	case d%time.Second == 0:
		return IntervalOf(int64(d/time.Second), IntervalSecond)
	case d%time.Millisecond == 0:
		return IntervalOf(int64(d/time.Millisecond), IntervalMillisecond)
	default:
		return IntervalOf(d.Microseconds(), IntervalMicrosecond)
	}
}

// IntervalExpr is an interval whose count comes from an integer expression:
// a column, or momo.Value(n) to bind a parameter. On PostgreSQL the count is
// multiplied into a one-unit INTERVAL. Bind a duration as
// IntervalExpr(momo.Value(d.Milliseconds()), IntervalMillisecond).
func IntervalExpr(count momo.Expression[int64], unit IntervalUnit) Interval {
	return Interval{count: count, unit: unit, isBuilt: true, isExpr: true}
}

// AddInterval is ts + iv: the timestamp moved forward by the interval.
// PostgreSQL renders (ts + INTERVAL ...), MySQL DATE_ADD and SQLite
// strftime with a modifier. The SQLite result is text in momo's timestamp
// format, in UTC, with millisecond precision. Month and year arithmetic
// differs at month ends: PostgreSQL and MySQL clamp to the last day (Jan 31
// plus one month is Feb 28 or 29), SQLite normalises the overflow into the
// next month (Mar 2 or 3). SQLite years outside 0000 to 9999 are NULL.
func AddInterval(ts momo.Expression[time.Time], iv Interval) Expr { return intervalExpr(ts, iv, 1) }

// SubInterval is ts - iv: the timestamp moved back by the interval.
func SubInterval(ts momo.Expression[time.Time], iv Interval) Expr { return intervalExpr(ts, iv, -1) }

// AddDuration is ts + d for a time.Duration, written as a literal interval
// (see IntervalDuration). It keeps sub-second precision where the dialect
// does: PostgreSQL and MySQL to the microsecond, SQLite to the millisecond.
func AddDuration(ts momo.Expression[time.Time], d time.Duration) Expr {
	return intervalExpr(ts, IntervalDuration(d), 1)
}

// SubDuration is ts - d for a time.Duration; see AddDuration.
func SubDuration(ts momo.Expression[time.Time], d time.Duration) Expr {
	return intervalExpr(ts, IntervalDuration(d), -1)
}

// decimalCount renders v / 10^digits as a plain decimal with exactly digits
// fraction digits, using integer arithmetic only.
func decimalCount(v int64, digits int) string {
	if digits == 0 {
		return strconv.FormatInt(v, 10)
	}
	div := int64(1)
	for i := 0; i < digits; i++ {
		div *= 10
	}
	sign := ""
	u := v
	if v < 0 {
		sign, u = "-", -v
	}
	frac := strconv.FormatInt(u%div, 10)
	for len(frac) < digits {
		frac = "0" + frac
	}
	return sign + strconv.FormatInt(u/div, 10) + "." + frac
}

func intervalExpr(ts momo.Expression[time.Time], iv Interval, sign int64) Expr {
	return newExpr(func(b *momo.Builder) {
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
		case !iv.isExpr && (iv.lit > spec.maxMag || iv.lit < -spec.maxMag):
			b.SetErr(timeErr("interval of %d %ss is outside the supported range", iv.lit, string(iv.unit)))
			return
		}
		switch b.Dialect().Name {
		case dialect.Postgres:
			b.WriteString("(")
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
			switch {
			case !iv.isExpr:
				b.WriteString(strconv.FormatInt(iv.lit*spec.myFactor, 10))
			case spec.myFactor != 1:
				b.WriteString("(")
				iv.count.AppendSQL(b)
				b.WriteString(") * " + strconv.FormatInt(spec.myFactor, 10))
			default:
				iv.count.AppendSQL(b)
			}
			b.WriteString(" " + spec.my + ")")
		case dialect.SQLite:
			b.WriteString("strftime(" + sqliteTimeFmt + ", ")
			ts.AppendSQL(b)
			b.WriteString(", ")
			if !iv.isExpr {
				b.WriteString("'" + decimalCount(iv.lit*sign*spec.factor, spec.frac) + " " + spec.sqlite + "')")
				return
			}
			if spec.frac > 0 {
				// printf %f never writes an exponent, which a date modifier would reject.
				b.WriteString("printf('%." + strconv.Itoa(spec.frac) + "f', (")
				iv.count.AppendSQL(b)
				b.WriteString(") * " + strconv.FormatInt(sign, 10) + ".0 / 1" + strings.Repeat("0", spec.frac) + ".0) || ' " + spec.sqlite + "')")
				return
			}
			b.WriteString("CAST((")
			iv.count.AppendSQL(b)
			b.WriteString(") * " + strconv.FormatInt(sign*spec.factor, 10) + " AS TEXT) || ' " + spec.sqlite + "')")
		default:
			b.Unsupported("interval arithmetic")
		}
	})
}

// ---------------------------------------------------------- Difference

// Span is the difference of two timestamps. It is not a selectable value of
// its own, because MySQL and SQLite have no interval value; read it as
// seconds with EpochSeconds. Build it with Sub or Age.
type Span struct {
	a, b momo.Expression[time.Time]
	age  bool
}

// Sub is a - b: the time from b to a, negative when a is earlier. On
// PostgreSQL it is the interval (a - b); on MySQL and SQLite it exists to
// feed EpochSeconds.
func Sub(a, b momo.Expression[time.Time]) Span { return Span{a: a, b: b} }

// Age is age(a, b) on PostgreSQL: an interval of years, months and days that
// counts calendar units, so the age of two dates a month apart is "1 mon"
// whatever the month's length. For a length of time in seconds, use Sub.
// MySQL and SQLite have no age and fail with momo.ErrUnsupported when the
// query is built.
func Age(a, b momo.Expression[time.Time]) Span { return Span{a: a, b: b, age: true} }

// AppendSQL renders the interval (a - b) or age(a, b) on PostgreSQL. Other
// dialects have no interval value and fail with momo.ErrUnsupported; use
// EpochSeconds.
func (s Span) AppendSQL(b *momo.Builder) {
	if !s.valid(b) {
		return
	}
	if b.Dialect().Name != dialect.Postgres {
		b.Unsupported("an interval value")
		return
	}
	if s.age {
		b.WriteString("age(")
		appendTime(b, s.a)
		b.WriteString(", ")
		appendTime(b, s.b)
		b.WriteString(")")
		return
	}
	b.WriteString("(")
	appendTime(b, s.a)
	b.WriteString(" - ")
	appendTime(b, s.b)
	b.WriteString(")")
}

func (s Span) valid(b *momo.Builder) bool {
	if s.a == nil || s.b == nil {
		b.SetErr(timeErr("Sub and Age need two timestamp expressions"))
		return false
	}
	return true
}

// EpochExpr is the length of a Span in seconds as a float64. Build it with
// EpochSeconds.
type EpochExpr struct {
	momo.Ops[float64]
	s Span
}

// EpochSeconds is the length of s in seconds, with a fraction:
// extract(epoch FROM a - b) on PostgreSQL (cast to double precision),
// TIMESTAMPDIFF(MICROSECOND, b, a) / 1e6 on MySQL and the julianday
// difference times 86400 on SQLite, which is accurate to about a tenth of a
// millisecond. Use it for a duration computed in SQL, for example
// percentile_cont over EpochSeconds(Sub(finished, started)). A Span from Age
// is PostgreSQL only, and there EXTRACT(EPOCH FROM interval) counts a month as
// 30 days and a year as 365.25 days, so the epoch of an Age that spans months
// or years is not the real elapsed time; use Sub for that.
func EpochSeconds(s Span) EpochExpr {
	x := EpochExpr{s: s}
	x.Ops = momo.NewOps[float64](x)
	return x
}

// As names the expression in a select list.
func (x EpochExpr) As(alias string) momo.Aliased[float64] { return momo.As[float64](x, alias) }

// AppendSQL renders the epoch seconds for the dialect of b.
func (x EpochExpr) AppendSQL(b *momo.Builder) {
	if !x.s.valid(b) {
		return
	}
	switch b.Dialect().Name {
	case dialect.Postgres:
		b.WriteString("CAST(EXTRACT(EPOCH FROM ")
		x.s.AppendSQL(b)
		b.WriteString(") AS double precision)")
	case dialect.MySQL:
		if x.s.age {
			b.Unsupported("Age")
			return
		}
		b.WriteString("(TIMESTAMPDIFF(MICROSECOND, ")
		x.s.b.AppendSQL(b)
		b.WriteString(", ")
		x.s.a.AppendSQL(b)
		b.WriteString(") / 1000000.0 + 0e0)")
	case dialect.SQLite:
		if x.s.age {
			b.Unsupported("Age")
			return
		}
		b.WriteString("((julianday(")
		x.s.a.AppendSQL(b)
		b.WriteString(") - julianday(")
		x.s.b.AppendSQL(b)
		b.WriteString(")) * 86400.0)")
	default:
		b.Unsupported("EpochSeconds")
	}
}

// ----------------------------------------------------------------- Now

func nowExpr(name string, pg, my string) Expr {
	return newExpr(func(b *momo.Builder) {
		switch b.Dialect().Name {
		case dialect.Postgres:
			b.WriteString(pg)
		case dialect.MySQL:
			if my == "" {
				b.Unsupported(name)
				return
			}
			b.WriteString(my)
		case dialect.SQLite:
			b.WriteString("strftime(" + sqliteTimeFmt + ", 'now')")
		default:
			b.Unsupported(name)
		}
	})
}

// Now is the database clock: now() on PostgreSQL (the start of the
// transaction), NOW(6) on MySQL, and strftime of 'now' in momo's timestamp
// format on SQLite. It reads the database's time, not the WithClock option.
func Now() Expr { return nowExpr("Now", "now()", "NOW(6)") }

// CurrentTimestamp is CURRENT_TIMESTAMP, with microsecond precision on
// MySQL, and the same millisecond text as Now on SQLite.
func CurrentTimestamp() Expr {
	return nowExpr("CurrentTimestamp", "CURRENT_TIMESTAMP", "CURRENT_TIMESTAMP(6)")
}

// ClockTimestamp is the wall-clock time at the moment it is evaluated:
// clock_timestamp() on PostgreSQL, which changes within a statement and a
// transaction, unlike Now. Use it to sample database time after a row lock is
// acquired (a lease or fence deadline).
//
// Other dialects differ: SQLite's value is fixed per statement, like Now.
// MySQL renders SYSDATE(6), the time the call runs, but only when the server
// is not started with sysdate-is-now, which makes SYSDATE behave like NOW.
func ClockTimestamp() Expr {
	return nowExpr("ClockTimestamp", "clock_timestamp()", "SYSDATE(6)")
}

// StatementTimestamp is the time the current statement started:
// statement_timestamp() on PostgreSQL, which differs from Now inside a
// transaction that runs several statements. MySQL has no such function and
// returns momo.ErrUnsupported when the query is built; SQLite renders the same
// millisecond text as Now, which is fixed per statement there.
func StatementTimestamp() Expr { return nowExpr("StatementTimestamp", "statement_timestamp()", "") }

// ---------------------------------------------------------------- Trunc

// TruncUnit is the precision of Trunc. Use the constants; any other
// value fails the build with momo.ErrValidation.
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

// Trunc rounds a timestamp down to the start of its unit:
// date_trunc('day', e) on PostgreSQL, and the equivalent strftime or
// DATE_FORMAT composition on SQLite and MySQL. The unit is written as a
// literal so SELECT and GROUP BY render identically. On PostgreSQL a
// timestamptz is truncated in the session time zone; wrap the input in
// AtTimeZone for buckets in a chosen zone. On SQLite the result
// is text in momo's timestamp format; on MySQL it is a DATETIME.
func Trunc(unit TruncUnit, e momo.Expression[time.Time]) Expr {
	return newExpr(func(b *momo.Builder) {
		f, ok := truncFormats[unit]
		switch {
		case !ok:
			b.SetErr(timeErr("truncation unit %q is not one of the Trunc... constants", string(unit)))
			return
		case e == nil:
			b.SetErr(timeErr("Trunc needs a timestamp expression"))
			return
		}
		switch b.Dialect().Name {
		case dialect.Postgres:
			b.WriteString("date_trunc('" + string(unit) + "', ")
			appendTime(b, e)
			b.WriteString(")")
		case dialect.SQLite:
			truncSQLite(b, unit, f.sqlite, e)
		case dialect.MySQL:
			truncMySQL(b, unit, f.mysql, e)
		default:
			b.Unsupported("Trunc")
		}
	})
}

func truncSQLite(b *momo.Builder, unit TruncUnit, layout string, e momo.Expr) {
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
		b.WriteString(")")
	}
}

func truncMySQL(b *momo.Builder, unit TruncUnit, layout string, e momo.Expr) {
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
