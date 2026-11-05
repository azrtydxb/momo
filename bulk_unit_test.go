// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/suite"
)

// smallLimit is a fake driver whose dialect allows only max bind parameters.
type smallLimit struct {
	*fakedriver.Driver
	d *dialect.Dialect
}

func (s smallLimit) Dialect() *dialect.Dialect { return s.d }

func newSmallLimit(max int) smallLimit {
	d := *dialect.For(dialect.Postgres)
	d.MaxParams = max
	return smallLimit{Driver: fakedriver.New(dialect.Postgres), d: &d}
}

// bulkOmit leaves 3 written columns: tenant_id, title, version, created_at,
// updated_at, deleted_at minus deleted_at = 5 bound columns per row.
func bulkQuery(db *momo.DB, n int) momo.InsertQuery[suite.Document] {
	ms := make([]suite.Document, n)
	for i := range ms {
		ms[i] = suite.Document{TenantID: int64(i), Title: "b"}
	}
	return momo.Insert[suite.Document](db).Omit(suite.DocBody, suite.DocSecret, suite.DocDeletedAt).Models(ms)
}

func TestBulkChunkBoundaries(t *testing.T) {
	const cols, max = 5, 50 // 10 rows per statement
	for _, tc := range []struct {
		name string
		rows int
		want []int // rows per statement
	}{
		{"below limit", 9, []int{9}},
		{"exactly at limit", 10, []int{10}},
		{"limit plus one", 11, []int{10, 1}},
		{"two full chunks", 20, []int{10, 10}},
		{"two full plus one", 21, []int{10, 10, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			drv := newSmallLimit(max)
			db := momo.New(drv)
			if _, err := bulkQuery(db, tc.rows).Exec(t.Context()); err != nil {
				t.Fatal(err)
			}
			calls := drv.Calls()
			if len(calls) != len(tc.want) {
				t.Fatalf("%d statements, want %d", len(calls), len(tc.want))
			}
			for i, c := range calls {
				if len(c.Args) != tc.want[i]*cols || len(c.Args) > max {
					t.Errorf("statement %d binds %d args, want %d (limit %d)", i, len(c.Args), tc.want[i]*cols, max)
				}
			}
			wantTx := 0
			if len(tc.want) > 1 {
				wantTx = 1
			}
			if drv.Commits() != wantTx {
				t.Errorf("commits = %d, want %d: chunks must share one transaction", drv.Commits(), wantTx)
			}
			// rows keep their order across chunks
			next := int64(0)
			for _, c := range calls {
				for r := 0; r < len(c.Args)/cols; r++ {
					if c.Args[r*cols] != next {
						t.Fatalf("row order broken at %d", next)
					}
					next++
				}
			}
		})
	}
}

func TestBulkSingleRowOverLimit(t *testing.T) {
	drv := newSmallLimit(4) // a row binds 5 parameters
	db := momo.New(drv)
	_, err := bulkQuery(db, 3).Exec(t.Context())
	if !errors.Is(err, momo.ErrLimitExceeded) {
		t.Fatalf("err = %v, want ErrLimitExceeded", err)
	}
	if n := len(drv.Calls()); n != 0 {
		t.Fatalf("%d statements ran, want none", n)
	}
}

func TestBulkChunksShareOneTimestamp(t *testing.T) {
	drv := newSmallLimit(50)
	reads := 0
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	db := momo.New(drv, momo.WithClock(func() time.Time { reads++; return base.Add(time.Duration(reads) * time.Hour) }))
	if _, err := bulkQuery(db, 25).Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
	if reads != 1 {
		t.Fatalf("clock read %d times, want once for the whole Exec", reads)
	}
	var ts any
	for _, c := range drv.Calls() {
		for r := 0; r < len(c.Args)/5; r++ {
			got := c.Args[r*5+3] // created_at
			if ts == nil {
				ts = got
			}
			if got != ts {
				t.Fatalf("created_at %v differs from %v across chunks", got, ts)
			}
		}
	}
}

func TestBulkUpsertChunksKeepIdentityConsistent(t *testing.T) {
	drv := newSmallLimit(60) // 6 bound columns with identity: 10 rows per statement
	db := momo.New(drv)
	ms := make([]suite.Document, 25)
	for i := range ms {
		ms[i] = suite.Document{ID: int64(i + 1), TenantID: 1, Title: "u"}
	}
	if _, err := momo.Upsert[suite.Document](db).Omit(suite.DocBody, suite.DocSecret, suite.DocDeletedAt).Models(ms).Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
	calls := drv.Calls()
	if len(calls) != 3 {
		t.Fatalf("%d statements, want 3", len(calls))
	}
	for i, c := range calls {
		if len(c.Args) > 60 {
			t.Errorf("statement %d binds %d args", i, len(c.Args))
		}
		for _, frag := range []string{`OVERRIDING SYSTEM VALUE`, `("id", `, `ON CONFLICT ("id") DO UPDATE SET`} {
			if !strings.Contains(c.SQL, frag) {
				t.Errorf("statement %d lacks %q: %s", i, frag, c.SQL)
			}
		}
	}
}

func TestBulkRowsReturnsChunksInOrder(t *testing.T) {
	drv := newSmallLimit(50)
	db := momo.New(drv)
	drv.SetRows([][]any{{int64(1)}, {int64(2)}})
	rows, err := bulkQuery(db, 25).Returning(suite.DocID).Rows(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for rows.Next() {
		n++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if len(drv.Calls()) != 3 || n != 6 { // 3 chunks x 2 scripted rows
		t.Fatalf("statements = %d, rows = %d, want 3 and 6", len(drv.Calls()), n)
	}
	if drv.Commits() != 1 {
		t.Fatalf("commits = %d, want 1", drv.Commits())
	}
}

func TestBulkRowsEarlyCloseRunsRemainingChunks(t *testing.T) {
	drv := newSmallLimit(50)
	db := momo.New(drv)
	rows, err := bulkQuery(db, 25).Returning(suite.DocID).Rows(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if len(drv.Calls()) != 3 || drv.Commits() != 1 {
		t.Fatalf("statements = %d commits = %d, want 3 and 1", len(drv.Calls()), drv.Commits())
	}
}

func TestBulkChunksOnCallerTxUseSavepoint(t *testing.T) {
	drv := newSmallLimit(50)
	db := momo.New(drv)
	tx, err := db.Begin(t.Context(), momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ms := make([]suite.Document, 25)
	_, err = momo.Insert[suite.Document](tx).Omit(suite.DocBody, suite.DocSecret, suite.DocDeletedAt).Models(ms).Exec(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var sp, rel int
	for _, c := range drv.Calls() {
		switch {
		case strings.HasPrefix(c.SQL, "SAVEPOINT"):
			sp++
		case strings.HasPrefix(c.SQL, "RELEASE"):
			rel++
		}
	}
	if sp != 1 || rel != 1 {
		t.Fatalf("savepoints = %d releases = %d, want one scope around the chunks", sp, rel)
	}
	_ = tx.Rollback(t.Context())
}
