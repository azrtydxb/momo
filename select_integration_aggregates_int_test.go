// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/types"
)

var (
	aiID = momo.NewColumn[int64]("agg_ints", "id", 0)
	aiA  = momo.NewColumn[int16]("agg_ints", "a", 0)
	aiB  = momo.NewColumn[int32]("agg_ints", "b", 0)
	aiC  = momo.NewColumn[int64]("agg_ints", "c", 0)
	aiN  = momo.NewColumn[types.Numeric]("agg_ints", "n", 0)

	aiInt = momo.NewColumn[int]("agg_ints", "b", 0)
	aiI8  = momo.NewColumn[int8]("agg_ints", "a", 0)

	abC = momo.NewColumn[int64]("agg_big", "c", 0)
)

// eachAggDB runs fn on PostgreSQL, SQLite, MySQL 8.4, MySQL 9.7 and MariaDB,
// one subtest each. testopen.Each covers only the first two; the MySQL servers
// are opened by name and skipped when their URL is unset.
func eachAggDB(t *testing.T, fn func(t *testing.T, db *momo.DB, d dialect.Name)) {
	t.Helper()
	for _, s := range []struct {
		name string
		d    dialect.Name
		open func(*testing.T, ...momo.Option) *momo.DB
	}{
		{"postgres", dialect.Postgres, testopen.Postgres},
		{"sqlite", dialect.SQLite, testopen.SQLite},
		{"mysql84", dialect.MySQL, testopen.MySQL84},
		{"mysql97", dialect.MySQL, testopen.MySQL97},
		{"mariadb118", dialect.MySQL, testopen.MariaDB},
	} {
		t.Run(s.name, func(t *testing.T) {
			fn(t, s.open(t), s.d)
		})
	}
}

// seedAggInts creates agg_ints with four rows: a is 1..4 (int16), b is
// 2000000000 each (int32, so the sum passes int32), c is 3000000000 each
// (int64) and n is 1.10, 2.25, 0.05 and 0.00 (types.Numeric). It also creates
// agg_big whose c column sums past int64.
func seedAggInts(t *testing.T, db *momo.DB, d dialect.Name) {
	t.Helper()
	num := "TEXT"
	if d == dialect.Postgres {
		num = "numeric"
	}
	for _, s := range []string{
		"DROP TABLE IF EXISTS agg_ints",
		"CREATE TABLE agg_ints (id bigint primary key, a smallint not null, b integer not null, c bigint not null, n " + num + " not null)",
		"INSERT INTO agg_ints VALUES (1, 1, 2000000000, 3000000000, '1.10')",
		"INSERT INTO agg_ints VALUES (2, 2, 2000000000, 3000000000, '2.25')",
		"INSERT INTO agg_ints VALUES (3, 3, 2000000000, 3000000000, '0.05')",
		"INSERT INTO agg_ints VALUES (4, 4, 2000000000, 3000000000, '0.00')",
		"DROP TABLE IF EXISTS agg_big",
		"CREATE TABLE agg_big (id bigint primary key, c bigint not null)",
		"INSERT INTO agg_big VALUES (1, 9000000000000000000)",
		"INSERT INTO agg_big VALUES (2, 9000000000000000000)",
	} {
		if _, err := db.Driver().Exec(context.Background(), s, nil); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

// aggScalar reads one aggregate over table where p holds, as a nullable T.
func aggScalar[T any](db *momo.DB, table string, e momo.Expr, p momo.Predicate) (*T, error) {
	r, err := momo.Select[momo.Row1[*T]](db).From(momo.TableRef{Name: table}).Columns(e).
		Where(p).One(context.Background())
	if err != nil {
		return nil, err
	}
	return r.C1, nil
}

// normDec drops the trailing zeros of a decimal's fraction, so "3.40" and
// "3.400000000000000000000000000000" compare equal to "3.4".
func normDec(s string) string {
	if strings.Contains(s, ".") {
		s = strings.TrimSuffix(strings.TrimRight(s, "0"), ".")
	}
	return s
}

func wantInt(t *testing.T, label string, got *int64, err error, want int64) {
	t.Helper()
	if err != nil {
		t.Errorf("%s: %v", label, err)
		return
	}
	if got == nil || *got != want {
		t.Errorf("%s = %v, want %d", label, got, want)
	}
}

func wantFloat(t *testing.T, label string, got *float64, err error, want float64) {
	t.Helper()
	if err != nil {
		t.Errorf("%s: %v", label, err)
		return
	}
	if got == nil || math.Abs(*got-want) > 1e-9 {
		t.Errorf("%s = %v, want %v", label, got, want)
	}
}

func wantNumeric(t *testing.T, label string, got *types.Numeric, err error, want string) {
	t.Helper()
	if err != nil {
		t.Errorf("%s: %v", label, err)
		return
	}
	if got == nil || normDec(got.String()) != want {
		t.Errorf("%s = %v, want %s", label, got, want)
	}
}

// Catches: SumInt rejecting or mis-scanning an int16, int32 or int64 column on
// a dialect (PostgreSQL numeric for SUM(bigint), MySQL DECIMAL), a 32-bit
// truncation of a sum past int32, and an empty set that scans as 0 instead of
// NULL.
func TestSumIntIntegration(t *testing.T) {
	eachAggDB(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedAggInts(t, db, d)
		all := aiID.Gt(0)
		got, err := aggScalar[int64](db, "agg_ints", momo.SumInt(aiA), all)
		wantInt(t, "SumInt(int16)", got, err, 10)
		got, err = aggScalar[int64](db, "agg_ints", momo.SumInt(aiB), all)
		wantInt(t, "SumInt(int32)", got, err, 8_000_000_000)
		got, err = aggScalar[int64](db, "agg_ints", momo.SumInt(aiC), all)
		wantInt(t, "SumInt(int64)", got, err, 12_000_000_000)

		empty := aiID.Eq(99)
		for name, e := range map[string]momo.Expr{
			"SumInt(int16)": momo.SumInt(aiA), "SumInt(int32)": momo.SumInt(aiB), "SumInt(int64)": momo.SumInt(aiC),
		} {
			got, err := aggScalar[int64](db, "agg_ints", e, empty)
			if err != nil || got != nil {
				t.Errorf("%s over no rows = %v, %v; want nil", name, got, err)
			}
		}
	})
}

// Catches: a sum past int64 wrapping or saturating silently instead of
// failing, and SumNumeric losing the digits SumInt cannot hold.
func TestSumIntOverflowIntegration(t *testing.T) {
	eachAggDB(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedAggInts(t, db, d)
		all := momo.SQL("1 = 1")
		got64, err64 := aggScalar[int64](db, "agg_big", momo.SumInt(abC), all)
		if err64 == nil {
			t.Errorf("SumInt past int64 = %v, want an error", got64)
		} else if d == dialect.MySQL && !strings.Contains(err64.Error(), "mysql: cannot convert the column value to the destination type") {
			t.Errorf("SumInt overflow error text = %q", err64.Error())
		}
		got, err := aggScalar[types.Numeric](db, "agg_big", momo.SumNumeric(abC), all)
		if d == dialect.SQLite {
			// SQLite sums integers as int64 and raises "integer overflow".
			if err == nil {
				t.Errorf("SumNumeric past int64 on sqlite = %v, want an error", got)
			}
			return
		}

		wantNumeric(t, "SumNumeric(int64) past int64", got, err, "18000000000000000000")
	})
}

// Catches: SumNumeric returning a float (inexact) or a wrongly scaled value,
// the MySQL TEXT column summed as DOUBLE, and SQLite silently summing a
// types.Numeric column as a float instead of failing with ErrUnsupported.
func TestSumNumericIntegration(t *testing.T) {
	eachAggDB(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedAggInts(t, db, d)
		all := aiID.Gt(0)
		got, err := aggScalar[types.Numeric](db, "agg_ints", momo.SumNumeric(aiB), all)
		wantNumeric(t, "SumNumeric(int32)", got, err, "8000000000")
		got, err = aggScalar[types.Numeric](db, "agg_ints", momo.SumNumeric(aiA), all)
		wantNumeric(t, "SumNumeric(int16)", got, err, "10")

		got, err = aggScalar[types.Numeric](db, "agg_ints", momo.SumNumeric(aiN), all)
		if d == dialect.SQLite {
			if !errors.Is(err, momo.ErrUnsupported) {
				t.Errorf("SumNumeric(Numeric) on sqlite = %v, %v; want ErrUnsupported", got, err)
			}
		} else {
			wantNumeric(t, "SumNumeric(Numeric)", got, err, "3.4")
			if d == dialect.MySQL && got != nil {
				// MySQL and MariaDB cast the TEXT column to DECIMAL(65,30), so the
				// result keeps 30 decimal places, trailing zeros included.
				if want := "3.400000000000000000000000000000"; got.String() != want {
					t.Errorf("SumNumeric(Numeric) text = %q, want %q", got.String(), want)
				}
			}
		}

		empty, err := aggScalar[types.Numeric](db, "agg_ints", momo.SumNumeric(aiB), aiID.Eq(99))
		if err != nil || empty != nil {
			t.Errorf("SumNumeric over no rows = %v, %v; want nil", empty, err)
		}
	})
}

// Catches: Avg refusing an integer column, PostgreSQL's numeric average not
// scanning into a float64, MySQL's DECIMAL average not scanning, and an empty
// set that scans as 0.
func TestAvgIntegration(t *testing.T) {
	eachAggDB(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedAggInts(t, db, d)
		all := aiID.Gt(0)
		got, err := aggScalar[float64](db, "agg_ints", momo.Avg(aiA), all)
		wantFloat(t, "Avg(int16)", got, err, 2.5)
		got, err = aggScalar[float64](db, "agg_ints", momo.Avg(aiB), all)
		wantFloat(t, "Avg(int32)", got, err, 2_000_000_000)
		got, err = aggScalar[float64](db, "agg_ints", momo.Avg(aiC), all)
		wantFloat(t, "Avg(int64)", got, err, 3_000_000_000)
		got, err = aggScalar[float64](db, "agg_ints", momo.Avg(aiN), all)
		wantFloat(t, "Avg(Numeric)", got, err, 0.85)

		empty, err := aggScalar[float64](db, "agg_ints", momo.Avg(aiB), aiID.Eq(99))
		if err != nil || empty != nil {
			t.Errorf("Avg over no rows = %v, %v; want nil", empty, err)
		}
	})
}

// Catches: AvgNumeric going through a float, the wrong scale or cast per
// dialect, and SQLite (whose AVG is always a float) returning an inexact
// average instead of ErrUnsupported.
func TestAvgNumericIntegration(t *testing.T) {
	eachAggDB(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedAggInts(t, db, d)
		all := aiID.Gt(0)
		if d == dialect.SQLite {
			for _, e := range []momo.Expr{momo.AvgNumeric(aiA), momo.AvgNumeric(aiN)} {
				if got, err := aggScalar[types.Numeric](db, "agg_ints", e, all); !errors.Is(err, momo.ErrUnsupported) {
					t.Errorf("AvgNumeric on sqlite = %v, %v; want ErrUnsupported", got, err)
				}
			}
			return
		}
		got, err := aggScalar[types.Numeric](db, "agg_ints", momo.AvgNumeric(aiA), all)
		wantNumeric(t, "AvgNumeric(int16)", got, err, "2.5")
		got, err = aggScalar[types.Numeric](db, "agg_ints", momo.AvgNumeric(aiC), all)
		wantNumeric(t, "AvgNumeric(int64)", got, err, "3000000000")
		got, err = aggScalar[types.Numeric](db, "agg_ints", momo.AvgNumeric(aiN), all)
		wantNumeric(t, "AvgNumeric(Numeric)", got, err, "0.85")

		empty, err := aggScalar[types.Numeric](db, "agg_ints", momo.AvgNumeric(aiA), aiID.Eq(99))
		if err != nil || empty != nil {
			t.Errorf("AvgNumeric over no rows = %v, %v; want nil", empty, err)
		}
	})
}

// Catches: the PostgreSQL cast of SumInt landing outside the FILTER or OVER
// clause (invalid SQL), and the window form losing the int64 result.
func TestSumIntFilterOverIntegration(t *testing.T) {
	eachAggDB(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedAggInts(t, db, d)
		got, err := aggScalar[int64](db, "agg_ints", momo.SumInt(aiA).Filter(aiA.Gt(1)), aiID.Gt(0))
		wantInt(t, "SumInt FILTER", got, err, 9)
		rows, err := momo.Select[momo.Row1[int64]](db).From(momo.TableRef{Name: "agg_ints"}).
			Columns(momo.SumInt(aiB).Over(momo.Over())).Where(aiID.Gt(0)).All(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 4 {
			t.Fatalf("window rows = %d, want 4", len(rows))
		}
		for _, r := range rows {
			if r.C1 != 8_000_000_000 {
				t.Errorf("SumInt OVER () = %d, want 8000000000", r.C1)
			}
		}
	})
}

// Catches: the numeric aggregates losing the exact result, or a cast landing
// outside the FILTER or OVER clause, on PostgreSQL, SQLite, MySQL and MariaDB.
// MySQL and MariaDB have no FILTER, so the predicate moves into the argument.
func TestNumericFilterOverIntegration(t *testing.T) {
	eachAggDB(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedAggInts(t, db, d)
		all := aiID.Gt(0)
		lt3 := aiA.Lt(3)
		cases := []struct {
			name   string
			e      momo.Expr
			want   string
			sqlite bool // SQLite can run it
		}{
			{"SumNumeric(int32) FILTER", momo.SumNumeric(aiB).Filter(aiA.Gt(1)), "6000000000", true},
			{"SumNumeric(Numeric) FILTER", momo.SumNumeric(aiN).Filter(aiA.Gt(1)), "2.3", false},
			{"AvgNumeric(int64) FILTER", momo.AvgNumeric(aiC).Filter(lt3), "3000000000", false},
			{"AvgNumeric(Numeric) FILTER", momo.AvgNumeric(aiN).Filter(lt3), "1.675", false},
		}
		for _, c := range cases {
			got, err := aggScalar[types.Numeric](db, "agg_ints", c.e, all)
			if d == dialect.SQLite && !c.sqlite {
				if !errors.Is(err, momo.ErrUnsupported) {
					t.Errorf("%s on sqlite = %v, %v; want ErrUnsupported", c.name, got, err)
				}
				continue
			}
			wantNumeric(t, c.name, got, err, c.want)
		}

		win := []struct {
			name   string
			e      momo.Expr
			want   string
			sqlite bool
		}{
			{"SumNumeric(int32) OVER", momo.SumNumeric(aiB).Over(momo.Over()), "8000000000", true},
			{"SumNumeric(Numeric) OVER", momo.SumNumeric(aiN).Over(momo.Over()), "3.4", false},
			{"SumNumeric(Numeric) FILTER OVER", momo.SumNumeric(aiN).Filter(lt3).Over(momo.Over()), "3.35", false},
			{"AvgNumeric(int16) OVER", momo.AvgNumeric(aiA).Over(momo.Over()), "2.5", false},
			{"AvgNumeric(Numeric) FILTER OVER", momo.AvgNumeric(aiN).Filter(lt3).Over(momo.Over()), "1.675", false},
		}
		for _, c := range win {
			rows, err := momo.Select[momo.Row1[types.Numeric]](db).From(momo.TableRef{Name: "agg_ints"}).
				Columns(c.e).Where(all).All(context.Background())
			if d == dialect.SQLite && !c.sqlite {
				if !errors.Is(err, momo.ErrUnsupported) {
					t.Errorf("%s on sqlite = %v, %v; want ErrUnsupported", c.name, rows, err)
				}
				continue
			}
			if err != nil {
				t.Errorf("%s: %v", c.name, err)
				continue
			}
			if len(rows) != 4 {
				t.Errorf("%s: %d rows, want 4", c.name, len(rows))
			}
			for _, r := range rows {
				if normDec(r.C1.String()) != c.want {
					t.Errorf("%s = %s, want %s", c.name, r.C1, c.want)
				}
			}
		}
	})
}

// Catches: SumInt, SumNumeric, Avg and AvgNumeric refusing a Go int or int8
// column, or mis-scanning its aggregate, on a dialect.
func TestIntAndInt8AggregatesIntegration(t *testing.T) {
	eachAggDB(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedAggInts(t, db, d)
		all := aiID.Gt(0)
		i, err := aggScalar[int64](db, "agg_ints", momo.SumInt(aiInt), all)
		wantInt(t, "SumInt(int)", i, err, 8_000_000_000)
		i, err = aggScalar[int64](db, "agg_ints", momo.SumInt(aiI8), all)
		wantInt(t, "SumInt(int8)", i, err, 10)
		n, err := aggScalar[types.Numeric](db, "agg_ints", momo.SumNumeric(aiInt), all)
		wantNumeric(t, "SumNumeric(int)", n, err, "8000000000")
		n, err = aggScalar[types.Numeric](db, "agg_ints", momo.SumNumeric(aiI8), all)
		wantNumeric(t, "SumNumeric(int8)", n, err, "10")
		f, err := aggScalar[float64](db, "agg_ints", momo.Avg(aiInt), all)
		wantFloat(t, "Avg(int)", f, err, 2_000_000_000)
		f, err = aggScalar[float64](db, "agg_ints", momo.Avg(aiI8), all)
		wantFloat(t, "Avg(int8)", f, err, 2.5)
		n, err = aggScalar[types.Numeric](db, "agg_ints", momo.AvgNumeric(aiI8), all)
		if d == dialect.SQLite {
			if !errors.Is(err, momo.ErrUnsupported) {
				t.Errorf("AvgNumeric(int8) on sqlite = %v, %v; want ErrUnsupported", n, err)
			}
			return
		}
		wantNumeric(t, "AvgNumeric(int8)", n, err, "2.5")
	})
}
