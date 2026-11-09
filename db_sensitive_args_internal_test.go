// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"slices"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func TestSensitiveTupleInAndPKPredicateMarkArgs(t *testing.T) {
	cases := map[string]struct {
		p    Predicate
		want []int
	}{
		"single column tuple": {inTuple{table: "t", cols: []string{"k"}, keys: [][]any{{"s1"}, {"s2"}}, flags: []uint16{FlagSensitive}}, []int{0, 1}},
		"composite row value": {inTuple{table: "t", cols: []string{"a", "k"}, keys: [][]any{{1, "s1"}, {2, "s2"}}, flags: []uint16{0, FlagSensitive}}, []int{1, 3}},
		"unflagged tuple":     {inTuple{table: "t", cols: []string{"k"}, keys: [][]any{{"s1"}}}, nil},
		"primary key":         {updPKEq{"t", "id", "s1", FlagPK | FlagSensitive}, []int{0}},
		"plain primary key":   {updPKEq{"t", "id", 1, FlagPK}, nil},
	}
	for _, d := range []*dialect.Dialect{dialect.For(dialect.Postgres), dialect.For(dialect.SQLite)} {
		for name, c := range cases {
			t.Run(string(d.Name)+"/"+name, func(t *testing.T) {
				comp, err := compile(d, func(b *Builder) { c.p.AppendSQL(b) })
				if err != nil {
					t.Fatal(err)
				}
				if !slices.Equal(comp.sens, c.want) {
					t.Fatalf("sens %v, want %v", comp.sens, c.want)
				}
			})
		}
	}
}
