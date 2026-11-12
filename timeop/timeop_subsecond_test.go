// SPDX-License-Identifier: Apache-2.0

package timeop_test

import (
	"errors"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/timeop"
)

var tmUpdated = momo.NewColumn[time.Time]("documents", "updated_at", 0)

const tmU = `"documents"."updated_at"`

// Catches: millisecond and microsecond counts dropped, rounded to seconds or
// left unscaled where the dialect has no such unit (MySQL counts microseconds,
// SQLite counts fractional seconds), and an exponent-style or lossy SQLite
// modifier for a bound count.
func TestSubSecondIntervalGolden(t *testing.T) {
	const sqliteFmt = `'%Y-%m-%d %H:%M:%f'`
	ms := timeop.IntervalOf(1500, timeop.IntervalMillisecond)
	us := timeop.IntervalOf(250, timeop.IntervalMicrosecond)
	bound := timeop.IntervalExpr(momo.Value(int64(40)), timeop.IntervalMillisecond)
	boundUs := timeop.IntervalExpr(momo.Value(int64(7)), timeop.IntervalMicrosecond)
	runTimeGoldens(t, []timeGolden{
		{dialect.Postgres, timeop.AddInterval(tmCreated, ms), `(` + tmC + ` + INTERVAL '1500 millisecond')`, nil},
		{dialect.Postgres, timeop.SubInterval(tmCreated, us), `(` + tmC + ` - INTERVAL '250 microsecond')`, nil},
		{dialect.Postgres, timeop.AddInterval(tmCreated, bound), `(` + tmC + ` + (CAST($1 AS bigint) * INTERVAL '1 millisecond'))`, []any{int64(40)}},
		{dialect.MySQL, timeop.AddInterval(tmCreated, ms), `DATE_ADD(` + tmC + `, INTERVAL 1500000 MICROSECOND)`, nil},
		{dialect.MySQL, timeop.SubInterval(tmCreated, us), `DATE_SUB(` + tmC + `, INTERVAL 250 MICROSECOND)`, nil},
		{dialect.MySQL, timeop.AddInterval(tmCreated, bound), `DATE_ADD(` + tmC + `, INTERVAL (?) * 1000 MICROSECOND)`, []any{int64(40)}},
		{dialect.MySQL, timeop.AddInterval(tmCreated, boundUs), `DATE_ADD(` + tmC + `, INTERVAL ? MICROSECOND)`, []any{int64(7)}},
		{dialect.SQLite, timeop.AddInterval(tmCreated, ms), `strftime(` + sqliteFmt + `, ` + tmC + `, '1.500 seconds')`, nil},
		{dialect.SQLite, timeop.SubInterval(tmCreated, ms), `strftime(` + sqliteFmt + `, ` + tmC + `, '-1.500 seconds')`, nil},
		{dialect.SQLite, timeop.SubInterval(tmCreated, timeop.IntervalOf(1, timeop.IntervalMillisecond)), `strftime(` + sqliteFmt + `, ` + tmC + `, '-0.001 seconds')`, nil},
		{dialect.SQLite, timeop.AddInterval(tmCreated, us), `strftime(` + sqliteFmt + `, ` + tmC + `, '0.000250 seconds')`, nil},
		{dialect.SQLite, timeop.AddInterval(tmCreated, bound), `strftime(` + sqliteFmt + `, ` + tmC + `, printf('%.3f', (?) * 1.0 / 1000.0) || ' seconds')`, []any{int64(40)}},
		{dialect.SQLite, timeop.SubInterval(tmCreated, boundUs), `strftime(` + sqliteFmt + `, ` + tmC + `, printf('%.6f', (?) * -1.0 / 1000000.0) || ' seconds')`, []any{int64(7)}},
	})
}

// Catches: a Duration rounded to whole seconds, a whole-second Duration that
// stops being a plain second interval, and a wrong sign for SubDuration.
func TestDurationGolden(t *testing.T) {
	const sqliteFmt = `'%Y-%m-%d %H:%M:%f'`
	runTimeGoldens(t, []timeGolden{
		{dialect.Postgres, timeop.AddDuration(tmCreated, 1500*time.Millisecond), `(` + tmC + ` + INTERVAL '1500 millisecond')`, nil},
		{dialect.Postgres, timeop.AddDuration(tmCreated, 2*time.Minute), `(` + tmC + ` + INTERVAL '120 second')`, nil},
		{dialect.Postgres, timeop.SubDuration(tmCreated, 1500*time.Microsecond), `(` + tmC + ` - INTERVAL '1500 microsecond')`, nil},
		{dialect.Postgres, timeop.AddDuration(tmCreated, 2500*time.Nanosecond), `(` + tmC + ` + INTERVAL '2 microsecond')`, nil},
		{dialect.Postgres, timeop.AddDuration(tmCreated, -1500*time.Millisecond), `(` + tmC + ` + INTERVAL '-1500 millisecond')`, nil},
		{dialect.Postgres, timeop.AddDuration(tmCreated, time.Duration(1<<62)), `(` + tmC + ` + INTERVAL '4611686018427387 microsecond')`, nil},
		{dialect.MySQL, timeop.AddDuration(tmCreated, 1500*time.Millisecond), `DATE_ADD(` + tmC + `, INTERVAL 1500000 MICROSECOND)`, nil},
		{dialect.MySQL, timeop.SubDuration(tmCreated, 90*time.Second), `DATE_SUB(` + tmC + `, INTERVAL 90 SECOND)`, nil},
		{dialect.SQLite, timeop.AddDuration(tmCreated, 1500*time.Millisecond), `strftime(` + sqliteFmt + `, ` + tmC + `, '1.500 seconds')`, nil},
		{dialect.SQLite, timeop.SubDuration(tmCreated, 90*time.Second), `strftime(` + sqliteFmt + `, ` + tmC + `, '-90 seconds')`, nil},
	})
}

// Catches: the wrong operand order (b - a), a missing double cast, MySQL
// seconds truncated by TIMESTAMPDIFF's integer result, Age rendered where the
// dialect has none, and a nil operand that is not refused.
func TestTimestampDifferenceGolden(t *testing.T) {
	at := time.Unix(0, 0).UTC()
	runTimeGoldens(t, []timeGolden{
		{dialect.Postgres, timeop.Sub(tmCreated, tmUpdated), `(` + tmC + ` - ` + tmU + `)`, nil},
		{dialect.Postgres, timeop.Age(tmCreated, tmUpdated), `age(` + tmC + `, ` + tmU + `)`, nil},
		{dialect.Postgres, timeop.EpochSeconds(timeop.Sub(tmCreated, tmUpdated)), `CAST(EXTRACT(EPOCH FROM (` + tmC + ` - ` + tmU + `)) AS double precision)`, nil},
		{dialect.Postgres, timeop.EpochSeconds(timeop.Age(tmCreated, tmUpdated)), `CAST(EXTRACT(EPOCH FROM age(` + tmC + `, ` + tmU + `)) AS double precision)`, nil},
		{dialect.Postgres, timeop.EpochSeconds(timeop.Sub(momo.Value(at), tmCreated)), `CAST(EXTRACT(EPOCH FROM (CAST($1 AS timestamptz) - ` + tmC + `)) AS double precision)`, []any{at}},
		{dialect.MySQL, timeop.EpochSeconds(timeop.Sub(tmCreated, tmUpdated)), `(TIMESTAMPDIFF(MICROSECOND, ` + tmU + `, ` + tmC + `) / 1000000.0 + 0e0)`, nil},
		{dialect.SQLite, timeop.EpochSeconds(timeop.Sub(tmCreated, tmUpdated)), `((julianday(` + tmC + `) - julianday(` + tmU + `)) * 86400.0)`, nil},
	})
	for _, d := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		for name, e := range map[string]momo.Expr{
			"age":       timeop.EpochSeconds(timeop.Age(tmCreated, tmUpdated)),
			"sub value": timeop.Sub(tmCreated, tmUpdated),
		} {
			if _, err := selectOne(d, e); !errors.Is(err, momo.ErrUnsupported) {
				t.Errorf("%s %s err = %v, want ErrUnsupported", d, name, err)
			}
		}
	}
	for _, d := range typedDialects {
		for name, e := range map[string]momo.Expr{
			"nil a":   timeop.EpochSeconds(timeop.Sub(nil, tmCreated)),
			"nil b":   timeop.EpochSeconds(timeop.Sub(tmCreated, nil)),
			"nil age": timeop.EpochSeconds(timeop.Age(nil, nil)),
			"zero":    timeop.EpochSeconds(timeop.Span{}),
			"huge ms": timeop.AddInterval(tmCreated, timeop.IntervalOf(1<<62, timeop.IntervalMillisecond)),
			"huge us": timeop.SubInterval(tmCreated, timeop.IntervalOf(1<<62, timeop.IntervalMicrosecond)),
		} {
			if _, err := selectOne(d, e); !errors.Is(err, momo.ErrValidation) {
				t.Errorf("%s %s err = %v, want ErrValidation", d, name, err)
			}
		}
	}
}
