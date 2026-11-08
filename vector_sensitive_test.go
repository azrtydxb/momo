// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/vector"
)

// TestDistanceArgMaskedForSensitiveColumn requires the query vector of a
// distance expression to show as RedactedArg in hook events when the vector
// column is sensitive, and to stay visible for a plain column. The driver gets
// the real vector either way.
func TestDistanceArgMaskedForSensitiveColumn(t *testing.T) {
	cases := []struct {
		name  string
		flags uint16
		want  bool
	}{
		{"sensitive column", momo.FlagSensitive, true},
		{"plain column", 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := &obstest.Recorder{}
			drv := fakedriver.New(dialect.Postgres)
			db := momo.New(drv, momo.WithHook(rec), momo.WithArgLogging())
			col := momo.NewVectorColumn[vector.Vector]("documents", "emb", 3, c.flags)
			dist := momo.CosineDistance(col, vector.Vector{1, 0, 0})
			if _, err := momo.Select[suite.Document](db).OrderBy(dist.Asc()).All(t.Context()); err != nil {
				t.Fatal(err)
			}
			evs := rec.Events()
			calls := drv.Calls()
			if len(evs) == 0 || len(calls) == 0 || len(evs[len(evs)-1].Args) != 1 || len(calls[len(calls)-1].Args) != 1 {
				t.Fatalf("want one statement with one argument, got %d events, %d calls", len(evs), len(calls))
			}
			seen, sent := evs[len(evs)-1].Args[0], calls[len(calls)-1].Args[0]
			if sent == momo.RedactedArg {
				t.Fatal("the driver received the placeholder instead of the vector")
			}
			if got := seen == momo.RedactedArg; got != c.want {
				t.Fatalf("hook saw %v, masked = %v, want masked = %v", seen, got, c.want)
			}
		})
	}
}

// hookArgs runs q on a driver of the given dialect and returns the one
// argument the hook saw and the one the driver got.
func hookArgs(t *testing.T, d dialect.Name, build func(db *momo.DB) momo.SelectQuery[suite.Document]) (seen, sent any) {
	t.Helper()
	rec := &obstest.Recorder{}
	drv := fakedriver.New(d)
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

// TestSQLiteDistanceArgMasked covers the sqlite-vec rendering, including the
// bit-vector Hamming path that wraps both operands in vec_bit.
func TestSQLiteDistanceArgMasked(t *testing.T) {
	for _, flags := range []uint16{momo.FlagSensitive, 0} {
		masked := flags&momo.FlagSensitive != 0
		t.Run("cosine", func(t *testing.T) {
			col := momo.NewVectorColumn[vector.Vector]("documents", "emb", 3, flags)
			seen, sent := hookArgs(t, dialect.SQLite, func(db *momo.DB) momo.SelectQuery[suite.Document] {
				return momo.Select[suite.Document](db).OrderBy(momo.CosineDistance(col, vector.Vector{1, 0, 0}).Asc())
			})
			if sent == momo.RedactedArg || (seen == momo.RedactedArg) != masked {
				t.Fatalf("hook saw %v, driver got %v, want masked = %v", seen, sent, masked)
			}
		})
		t.Run("hamming", func(t *testing.T) {
			col := momo.NewVectorColumn[vector.Bits]("documents", "bits", 8, flags)
			seen, sent := hookArgs(t, dialect.SQLite, func(db *momo.DB) momo.SelectQuery[suite.Document] {
				return momo.Select[suite.Document](db).OrderBy(momo.HammingDistance(col, vector.Bits{Len: 8, Data: []byte{0xB0}}).Asc())
			})
			if sent == momo.RedactedArg || (seen == momo.RedactedArg) != masked {
				t.Fatalf("hook saw %v, driver got %v, want masked = %v", seen, sent, masked)
			}
		})
	}
}

// TestPostgresHammingArgMasked covers the bit-vector path on PostgreSQL.
func TestPostgresHammingArgMasked(t *testing.T) {
	col := momo.NewVectorColumn[vector.Bits]("documents", "bits", 8, momo.FlagSensitive)
	seen, sent := hookArgs(t, dialect.Postgres, func(db *momo.DB) momo.SelectQuery[suite.Document] {
		return momo.Select[suite.Document](db).OrderBy(momo.HammingDistance(col, vector.Bits{Len: 8, Data: []byte{0xB0}}).Asc())
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
			seen, sent := hookArgs(t, dialect.Postgres, func(db *momo.DB) momo.SelectQuery[suite.Document] {
				return momo.Select[suite.Document](db).Where(radius.GtExpr(momo.CosineDistance(vec, vector.Vector{1, 0, 0})))
			})
			if sent == momo.RedactedArg || seen != momo.RedactedArg {
				t.Fatalf("hook saw %v, driver got %v, want the placeholder for the hook only", seen, sent)
			}
		})
	}
}
