// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"sync"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

func TestQueryReuseNoLeak(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.Postgres))
	// Three separate Where calls leave the base predicate slice with spare
	// capacity (len 3, cap 4 without clipping), which is what exposes an
	// append into a shared backing array.
	base := momo.Select[unitDocument](db).
		Where(DocPublished.Eq(true)).
		Where(DocPublished.Eq(true)).
		Where(DocPublished.Eq(true)).
		OrderBy(DocID.Asc())
	before, err := base.SQL()
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			d := base.Where(DocTitle.Eq("t")).Limit(int64(i))
			if _, err := d.SQL(); err != nil {
				t.Error(err)
			}
			e := base.Where(DocTenantID.Eq(int64(i)))
			if _, err := e.SQL(); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	// Two queries derived from the same base must not see each other's predicate.
	a := base.Where(DocTitle.Eq("only-a"))
	b := base.Where(DocTenantID.Eq(int64(7)))
	aq, err := a.SQL()
	if err != nil {
		t.Fatal(err)
	}
	if len(aq.Args) != len(before.Args)+1 {
		t.Fatalf("derived query a has %d args, want %d", len(aq.Args), len(before.Args)+1)
	}
	if _, err := b.SQL(); err != nil {
		t.Fatal(err)
	}
	aq2, err := a.SQL()
	if err != nil {
		t.Fatal(err)
	}
	if aq.SQL != aq2.SQL || len(aq.Args) != len(aq2.Args) {
		t.Fatalf("derived query a changed after deriving b:\n%s\n%s", aq.SQL, aq2.SQL)
	}
	after, err := base.SQL()
	if err != nil {
		t.Fatal(err)
	}
	if before.SQL != after.SQL || len(after.Args) != len(before.Args) {
		t.Fatalf("base query changed:\n%s\n%s", before.SQL, after.SQL)
	}
}
