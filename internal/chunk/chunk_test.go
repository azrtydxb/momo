// SPDX-License-Identifier: Apache-2.0

package chunk

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func TestKeysPerStatementPerDialect(t *testing.T) {
	cases := []struct {
		name  dialect.Name
		ncols int
		want  int
	}{
		{dialect.Postgres, 1, 65535},
		{dialect.Postgres, 2, MaxCompositeTuples},
		{dialect.Postgres, 3, MaxCompositeTuples},
		{dialect.MySQL, 1, 65535},
		{dialect.MySQL, 2, MaxCompositeTuples},
		{dialect.SQLite, 1, 32766},
		{dialect.SQLite, 2, MaxCompositeTuplesSQLite},
		{dialect.SQLite, 5, MaxCompositeTuplesSQLite},
	}
	for _, c := range cases {
		if got := KeysPerStatement(dialect.For(c.name), c.ncols, 0); got != c.want {
			t.Errorf("%s ncols=%d: got %d, want %d", c.name, c.ncols, got, c.want)
		}
	}
}

func TestKeysPerStatementCompositeCapValues(t *testing.T) {
	if MaxCompositeTuples != 2000 || MaxCompositeTuplesSQLite != 400 {
		t.Fatalf("caps changed: %d and %d, want 2000 and 400", MaxCompositeTuples, MaxCompositeTuplesSQLite)
	}
}

func TestKeysPerStatementReservedAndLimits(t *testing.T) {
	tight := *dialect.For(dialect.Postgres)
	tight.MaxParams = 10
	cases := []struct {
		name     string
		d        *dialect.Dialect
		ncols    int
		reserved int
		want     int
	}{
		{"sqlite single column minus reserved", dialect.For(dialect.SQLite), 1, 16, 32766 - 16},
		{"sqlite composite is capped", dialect.For(dialect.SQLite), 2, 16, MaxCompositeTuplesSQLite},
		{"postgres single column minus reserved", dialect.For(dialect.Postgres), 1, 16, 65535 - 16},
		{"zero columns count as one", dialect.For(dialect.Postgres), 0, 0, 65535},
		{"over-reserved floors at one", dialect.For(dialect.SQLite), 2, 40000, 1},
		{"tight bind limit wins over the cap", &tight, 2, 0, 5},
		{"unlimited dialect, composite still capped", &dialect.Dialect{}, 2, 0, MaxCompositeTuples},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := KeysPerStatement(c.d, c.ncols, c.reserved); got != c.want {
				t.Fatalf("got %d, want %d", got, c.want)
			}
		})
	}
	if got := KeysPerStatement(&dialect.Dialect{}, 1, 0); got < 1<<20 {
		t.Fatalf("unlimited dialect, single column: got %d", got)
	}
}

func TestEachSplitsInOrder(t *testing.T) {
	var got [][2]int
	err := Each(7, 3, func(lo, hi int) error {
		got = append(got, [2]int{lo, hi})
		return nil
	})
	want := [][2]int{{0, 3}, {3, 6}, {6, 7}}
	if err != nil || len(got) != len(want) {
		t.Fatalf("got %v, %v, want %v", got, err, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("chunk %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestEachEdges(t *testing.T) {
	if err := Each(0, 3, func(int, int) error { t.Fatal("called for empty input"); return nil }); err != nil {
		t.Fatal(err)
	}
	calls := 0
	_ = Each(2, 0, func(lo, hi int) error { calls++; return nil })
	if calls != 2 {
		t.Fatalf("n below one acts as one: %d calls, want 2", calls)
	}
	boom := errors.New("boom")
	calls = 0
	err := Each(9, 3, func(int, int) error { calls++; return boom })
	if !errors.Is(err, boom) || calls != 1 {
		t.Fatalf("got %v after %d calls, want boom after 1", err, calls)
	}
}
