// SPDX-License-Identifier: Apache-2.0

package vecop_test

import (
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/vecop"
	"github.com/azrtydxb/momo/vector"
)

// mariaDriver is the recording fake driver reporting the MariaDB flavor.
type mariaDriver struct{ *fakedriver.Driver }

func (mariaDriver) Dialect() *dialect.Dialect { return dialect.MySQLFlavor(dialect.FlavorMariaDB) }

// recorded is a driver that keeps the statements it received.
type recorded interface {
	momo.Driver
	Calls() []fakedriver.Call
}

// hookArgs runs the query on a driver and returns the one argument the hook
// saw and the one the driver got.
func hookArgs(t *testing.T, drv recorded, build func(db *momo.DB) momo.SelectQuery[suite.Document]) (seen, sent any) {
	t.Helper()
	rec := &obstest.Recorder{}
	db := momo.New(drv, momo.WithHook(rec), momo.WithArgLogging())
	if _, err := build(db).All(t.Context()); err != nil {
		t.Fatal(err)
	}
	evs, calls := rec.Events(), drv.Calls()
	if len(evs) == 0 || len(calls) == 0 || len(evs[len(evs)-1].Args) != 1 || len(calls[len(calls)-1].Args) != 1 {
		t.Fatalf("want one statement with one argument, got %d events, %d calls", len(evs), len(calls))
	}
	return evs[len(evs)-1].Args[0], calls[len(calls)-1].Args[0]
}

// TestDistanceArgMaskedForSensitiveColumn requires the query vector of a
// distance expression to show as RedactedArg in hook events when the vector
// column is sensitive, and to stay visible for a plain column, on every
// dialect with a distance. The driver gets the real vector either way,
// through the MariaDB text-to-vector wrapper too.
func TestDistanceArgMaskedForSensitiveColumn(t *testing.T) {
	drivers := map[string]func() recorded{
		"postgres": func() recorded {
			return fakedriver.New(dialect.Postgres)
		},
		"sqlite": func() recorded {
			return fakedriver.New(dialect.SQLite)
		},
		"mariadb": func() recorded {
			return mariaDriver{fakedriver.New(dialect.MySQL)}
		},
	}
	for name, newDriver := range drivers {
		for _, flags := range []uint16{momo.FlagSensitive, 0} {
			masked := flags&momo.FlagSensitive != 0
			t.Run(name, func(t *testing.T) {
				col := momo.NewVectorColumn[vector.Vector]("documents", "emb", 3, flags)
				seen, sent := hookArgs(t, newDriver(), func(db *momo.DB) momo.SelectQuery[suite.Document] {
					return momo.Select[suite.Document](db).OrderBy(vecop.Cosine(col, vector.Vector{1, 0, 0}).Asc())
				})
				if sent == momo.RedactedArg || (seen == momo.RedactedArg) != masked {
					t.Fatalf("hook saw %v, driver got %v, want masked = %v", seen, sent, masked)
				}
			})
		}
	}
}

// TestSQLiteHammingArgMasked covers the sqlite-vec bit-vector path that wraps
// both operands in vec_bit.
func TestSQLiteHammingArgMasked(t *testing.T) {
	for _, flags := range []uint16{momo.FlagSensitive, 0} {
		masked := flags&momo.FlagSensitive != 0
		col := momo.NewVectorColumn[vector.Bits]("documents", "bits", 8, flags)
		seen, sent := hookArgs(t, fakedriver.New(dialect.SQLite), func(db *momo.DB) momo.SelectQuery[suite.Document] {
			return momo.Select[suite.Document](db).OrderBy(vecop.Hamming(col, vector.Bits{Len: 8, Data: []byte{0xB0}}).Asc())
		})
		if sent == momo.RedactedArg || (seen == momo.RedactedArg) != masked {
			t.Fatalf("hook saw %v, driver got %v, want masked = %v", seen, sent, masked)
		}
	}
}

// TestPostgresHammingArgMasked covers the bit-vector path on PostgreSQL.
func TestPostgresHammingArgMasked(t *testing.T) {
	col := momo.NewVectorColumn[vector.Bits]("documents", "bits", 8, momo.FlagSensitive)
	seen, sent := hookArgs(t, fakedriver.New(dialect.Postgres), func(db *momo.DB) momo.SelectQuery[suite.Document] {
		return momo.Select[suite.Document](db).OrderBy(vecop.Hamming(col, vector.Bits{Len: 8, Data: []byte{0xB0}}).Asc())
	})
	if sent == momo.RedactedArg || seen != momo.RedactedArg {
		t.Fatalf("hook saw %v, driver got %v, want the placeholder for the hook only", seen, sent)
	}
}

// TestDistanceInPredicateMasksWithSensitiveColumn: a distance compared to a
// column in a WHERE clause shows its query vector as RedactedArg when either
// the vector column or the compared column is sensitive.
func TestDistanceInPredicateMasksWithSensitiveColumn(t *testing.T) {
	for name, c := range map[string]struct{ vecFlags, scalarFlags uint16 }{
		"sensitive vector column": {momo.FlagSensitive, 0},
		"sensitive scalar column": {0, momo.FlagSensitive},
	} {
		t.Run(name, func(t *testing.T) {
			vec := momo.NewVectorColumn[vector.Vector]("documents", "emb", 3, c.vecFlags)
			radius := momo.NewColumn[float64]("documents", "radius", c.scalarFlags)
			seen, sent := hookArgs(t, fakedriver.New(dialect.Postgres), func(db *momo.DB) momo.SelectQuery[suite.Document] {
				return momo.Select[suite.Document](db).Where(radius.GtExpr(vecop.Cosine(vec, vector.Vector{1, 0, 0})))
			})
			if sent == momo.RedactedArg || seen != momo.RedactedArg {
				t.Fatalf("hook saw %v, driver got %v, want the placeholder for the hook only", seen, sent)
			}
		})
	}
}
