// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
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
