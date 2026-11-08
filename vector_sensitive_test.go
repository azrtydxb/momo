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
