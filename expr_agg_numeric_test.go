// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/types"
)

var (
	aggI16 = momo.NewColumn[int16]("t", "a", 0)
	aggI32 = momo.NewColumn[int32]("t", "b", 0)
	aggI64 = momo.NewColumn[int64]("t", "c", 0)
	aggNum = momo.NewColumn[types.Numeric]("t", "d", 0)
	aggF64 = momo.NewColumn[float64]("t", "e", 0)
	aggInt = momo.NewColumn[int]("t", "f", 0)
	aggI8  = momo.NewColumn[int8]("t", "g", 0)
)

func renderAgg(d dialect.Name, e momo.Expr) (string, error) {
	b := momo.NewBuilder(dialect.For(d))
	e.AppendSQL(b)
	return b.String(), b.Err()
}

// Catches: SumInt, SumNumeric, Avg and AvgNumeric writing the wrong cast for
// a dialect, a cast that lands outside FILTER or OVER (invalid SQL), and an
// exact aggregate that silently goes inexact on SQLite instead of failing.
func TestAggregatesIntegerAndExactRender(t *testing.T) {
	over := momo.Over().PartitionBy(aggI64)
	cases := []struct {
		name string
		d    dialect.Name
		e    momo.Expr
		want string
		// unsupported: the build must fail with ErrUnsupported.
		unsupported bool
	}{
		{"sumint int32 postgres", dialect.Postgres, momo.SumInt(aggI32), `CAST(SUM("t"."b") AS bigint)`, false},
		{"sumint int16 postgres", dialect.Postgres, momo.SumInt(aggI16), `CAST(SUM("t"."a") AS bigint)`, false},
		{"sumint int64 postgres", dialect.Postgres, momo.SumInt(aggI64), `CAST(SUM("t"."c") AS bigint)`, false},
		{"sumint sqlite", dialect.SQLite, momo.SumInt(aggI32), `SUM("t"."b")`, false},
		{"sumint mysql", dialect.MySQL, momo.SumInt(aggI32), "SUM(`t`.`b`)", false},
		{"sumint filter postgres", dialect.Postgres, momo.SumInt(aggI32).Filter(aggI64.Gt(1)),
			`CAST(SUM("t"."b") FILTER (WHERE "t"."c" > $1) AS bigint)`, false},
		{"sumint over postgres", dialect.Postgres, momo.SumInt(aggI32).Over(over),
			`CAST(SUM("t"."b") OVER (PARTITION BY "t"."c") AS bigint)`, false},
		{"sumint filter over postgres", dialect.Postgres, momo.SumInt(aggI32).Filter(aggI64.Gt(1)).Over(over),
			`CAST(SUM("t"."b") FILTER (WHERE "t"."c" > $1) OVER (PARTITION BY "t"."c") AS bigint)`, false},
		{"sumint over sqlite", dialect.SQLite, momo.SumInt(aggI32).Over(over),
			`SUM("t"."b") OVER (PARTITION BY "t"."c")`, false},

		{"sumnumeric int postgres", dialect.Postgres, momo.SumNumeric(aggI32), `SUM(CAST("t"."b" AS numeric))`, false},
		{"sumnumeric numeric postgres", dialect.Postgres, momo.SumNumeric(aggNum), `SUM("t"."d")`, false},
		{"sumnumeric int mysql", dialect.MySQL, momo.SumNumeric(aggI32), "SUM(`t`.`b`)", false},
		{"sumnumeric numeric mysql", dialect.MySQL, momo.SumNumeric(aggNum), "SUM(CAST(`t`.`d` AS DECIMAL(65,30)))", false},
		{"sumnumeric int sqlite", dialect.SQLite, momo.SumNumeric(aggI64), `SUM("t"."c")`, false},
		{"sumnumeric numeric sqlite", dialect.SQLite, momo.SumNumeric(aggNum), "", true},

		{"avg int postgres", dialect.Postgres, momo.Avg(aggI32), `CAST(AVG("t"."b") AS double precision)`, false},
		{"avg numeric postgres", dialect.Postgres, momo.Avg(aggNum), `CAST(AVG("t"."d") AS double precision)`, false},
		{"avg float postgres", dialect.Postgres, momo.Avg(aggF64), `AVG("t"."e")`, false},
		{"avg int sqlite", dialect.SQLite, momo.Avg(aggI32), `AVG("t"."b")`, false},
		{"avg int mysql", dialect.MySQL, momo.Avg(aggI32), "AVG(`t`.`b`)", false},
		{"avg over postgres", dialect.Postgres, momo.Avg(aggI32).Over(over),
			`CAST(AVG("t"."b") OVER (PARTITION BY "t"."c") AS double precision)`, false},

		{"avgnumeric int postgres", dialect.Postgres, momo.AvgNumeric(aggI32), `AVG(CAST("t"."b" AS numeric))`, false},
		{"avgnumeric numeric postgres", dialect.Postgres, momo.AvgNumeric(aggNum), `AVG("t"."d")`, false},
		{"avgnumeric int mysql", dialect.MySQL, momo.AvgNumeric(aggI32), "AVG(`t`.`b`)", false},
		{"avgnumeric numeric mysql", dialect.MySQL, momo.AvgNumeric(aggNum), "AVG(CAST(`t`.`d` AS DECIMAL(65,30)))", false},
		{"avgnumeric int sqlite", dialect.SQLite, momo.AvgNumeric(aggI32), "", true},
		{"avgnumeric numeric sqlite", dialect.SQLite, momo.AvgNumeric(aggNum), "", true},

		// FILTER and OVER on the numeric aggregates. PostgreSQL keeps the cast
		// around the call and its tail; MySQL and MariaDB have no FILTER and
		// move the predicate into the argument, keeping the DECIMAL cast inside.
		{"sumint filter mysql", dialect.MySQL, momo.SumInt(aggI32).Filter(aggI64.Gt(1)),
			"SUM(CASE WHEN `t`.`c` > ? THEN `t`.`b` END)", false},
		{"sumint filter over mysql", dialect.MySQL, momo.SumInt(aggI32).Filter(aggI64.Gt(1)).Over(over),
			"SUM(CASE WHEN `t`.`c` > ? THEN `t`.`b` END) OVER (PARTITION BY `t`.`c`)", false},
		{"sumnumeric numeric filter mysql", dialect.MySQL, momo.SumNumeric(aggNum).Filter(aggI64.Gt(1)),
			"SUM(CASE WHEN `t`.`c` > ? THEN CAST(`t`.`d` AS DECIMAL(65,30)) END)", false},
		{"sumnumeric int filter mysql", dialect.MySQL, momo.SumNumeric(aggI32).Filter(aggI64.Gt(1)),
			"SUM(CASE WHEN `t`.`c` > ? THEN `t`.`b` END)", false},
		{"sumnumeric numeric filter over mysql", dialect.MySQL, momo.SumNumeric(aggNum).Filter(aggI64.Gt(1)).Over(over),
			"SUM(CASE WHEN `t`.`c` > ? THEN CAST(`t`.`d` AS DECIMAL(65,30)) END) OVER (PARTITION BY `t`.`c`)", false},
		{"avgnumeric numeric filter mysql", dialect.MySQL, momo.AvgNumeric(aggNum).Filter(aggI64.Gt(1)),
			"AVG(CASE WHEN `t`.`c` > ? THEN CAST(`t`.`d` AS DECIMAL(65,30)) END)", false},
		{"avg int filter mysql", dialect.MySQL, momo.Avg(aggI32).Filter(aggI64.Gt(1)),
			"AVG(CASE WHEN `t`.`c` > ? THEN `t`.`b` END)", false},

		{"sumnumeric numeric filter postgres", dialect.Postgres, momo.SumNumeric(aggNum).Filter(aggI64.Gt(1)),
			`SUM("t"."d") FILTER (WHERE "t"."c" > $1)`, false},
		{"sumnumeric int filter postgres", dialect.Postgres, momo.SumNumeric(aggI32).Filter(aggI64.Gt(1)),
			`SUM(CAST("t"."b" AS numeric)) FILTER (WHERE "t"."c" > $1)`, false},
		{"sumnumeric numeric over postgres", dialect.Postgres, momo.SumNumeric(aggNum).Over(over),
			`SUM("t"."d") OVER (PARTITION BY "t"."c")`, false},
		{"sumnumeric int filter over postgres", dialect.Postgres, momo.SumNumeric(aggI32).Filter(aggI64.Gt(1)).Over(over),
			`SUM(CAST("t"."b" AS numeric)) FILTER (WHERE "t"."c" > $1) OVER (PARTITION BY "t"."c")`, false},
		{"avgnumeric numeric filter postgres", dialect.Postgres, momo.AvgNumeric(aggNum).Filter(aggI64.Gt(1)),
			`AVG("t"."d") FILTER (WHERE "t"."c" > $1)`, false},
		{"avgnumeric int over postgres", dialect.Postgres, momo.AvgNumeric(aggI32).Over(over),
			`AVG(CAST("t"."b" AS numeric)) OVER (PARTITION BY "t"."c")`, false},
		{"avgnumeric numeric filter over postgres", dialect.Postgres, momo.AvgNumeric(aggNum).Filter(aggI64.Gt(1)).Over(over),
			`AVG("t"."d") FILTER (WHERE "t"."c" > $1) OVER (PARTITION BY "t"."c")`, false},
		{"avg int filter postgres", dialect.Postgres, momo.Avg(aggI32).Filter(aggI64.Gt(1)),
			`CAST(AVG("t"."b") FILTER (WHERE "t"."c" > $1) AS double precision)`, false},

		// int and int8 columns.
		{"sumint int postgres", dialect.Postgres, momo.SumInt(aggInt), `CAST(SUM("t"."f") AS bigint)`, false},
		{"sumint int8 postgres", dialect.Postgres, momo.SumInt(aggI8), `CAST(SUM("t"."g") AS bigint)`, false},
		{"sumint int mysql", dialect.MySQL, momo.SumInt(aggInt), "SUM(`t`.`f`)", false},
		{"sumint int8 sqlite", dialect.SQLite, momo.SumInt(aggI8), `SUM("t"."g")`, false},
		{"sumnumeric int postgres 2", dialect.Postgres, momo.SumNumeric(aggInt), `SUM(CAST("t"."f" AS numeric))`, false},
		{"sumnumeric int8 sqlite", dialect.SQLite, momo.SumNumeric(aggI8), `SUM("t"."g")`, false},
		{"avgnumeric int8 mysql", dialect.MySQL, momo.AvgNumeric(aggI8), "AVG(`t`.`g`)", false},
		{"avg int postgres", dialect.Postgres, momo.Avg(aggInt), `CAST(AVG("t"."f") AS double precision)`, false},
		{"avg int8 sqlite", dialect.SQLite, momo.Avg(aggI8), `AVG("t"."g")`, false},
	}
	for _, c := range cases {
		got, err := renderAgg(c.d, c.e)
		if c.unsupported {
			if !errors.Is(err, momo.ErrUnsupported) {
				t.Errorf("%s: err = %v, want ErrUnsupported", c.name, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
}

// Catches: an ErrUnsupported on SQLite that does not say which aggregate
// failed or what to call instead, so the caller cannot fix the query from the
// message.
func TestExactAggregateSQLiteMessage(t *testing.T) {
	cases := []struct {
		name string
		e    momo.Expr
		want string
	}{
		{"sumnumeric", momo.SumNumeric(aggNum), "SumNumeric"},
		{"avgnumeric int", momo.AvgNumeric(aggI32), "AvgNumeric"},
		{"avgnumeric numeric", momo.AvgNumeric(aggNum), "AvgNumeric"},
	}
	for _, c := range cases {
		_, err := renderAgg(dialect.SQLite, c.e)
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s: err = %v, want ErrUnsupported", c.name, err)
			continue
		}
		for _, w := range []string{c.want, "Avg", "float64", "SumInt", "integer"} {
			if !strings.Contains(err.Error(), w) {
				t.Errorf("%s: error %q lacks %q", c.name, err, w)
			}
		}
	}
}
