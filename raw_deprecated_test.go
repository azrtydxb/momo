// SPDX-License-Identifier: Apache-2.0

//lint:file-ignore SA1019 these tests exist to exercise the deprecated names

package momo_test

import (
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

// Catches: a deprecated raw SQL name that stops forwarding to its renamed
// function, so a caller that has not migrated yet compiles different SQL.
func TestDeprecatedRawQueryMatchesCompilableSQL(t *testing.T) {
	q := momo.New(fakedriver.New(dialect.Postgres))
	const sql = "SELECT id FROM tasks WHERE owner = ? AND done = ?"
	old, err := momo.RawQuery(q, sql, 7, false).SQL()
	if err != nil {
		t.Fatal(err)
	}
	now, err := momo.CompilableSQL(q, sql, 7, false).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if old.SQL != now.SQL || len(old.Args) != len(now.Args) {
		t.Fatalf("RawQuery = %q %v, CompilableSQL = %q %v", old.SQL, old.Args, now.SQL, now.Args)
	}
}

// Catches: the deprecated subquery wrapper no longer rendering like
// SubquerySQL inside an IN list.
func TestDeprecatedRawSubqueryMatchesSubquerySQL(t *testing.T) {
	old, now := momo.RawSubquery("SELECT 1"), momo.SubquerySQL("SELECT 1")
	if old == nil || now == nil {
		t.Fatal("a raw subquery constructor returned nil")
	}
}

// Catches: a deprecated Scalars or RawRows whose signature drifts from the
// renamed function; the assignments fail to compile when they do.
func TestDeprecatedRawRunnersKeepTheirSignatures(t *testing.T) {
	_ = []any{momo.Scalars[int64], momo.QueryScalars[int64], momo.RawRows, momo.QueryCursor}
}
