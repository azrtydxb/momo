// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// joinRender compiles SELECT <e> FROM documents for d.
func joinRender(d dialect.Name, e momo.Expr) (momo.Compiled, error) {
	return momo.Select[unitTally](typedDB(d)).From(momo.TableRef{Name: "documents"}).Columns(e).SQL()
}

// Catches: parts that lose their arguments or numbering when joined, lost
// parentheses (an OR part regrouping an AND join), and a comma join that
// wraps its parts.
func TestJoinSQLGolden(t *testing.T) {
	a := momo.SQL("status = ? OR kind = ?", "x", "y")
	b := momo.SQL("owner_id = ?", int64(7))
	got, err := joinRender(dialect.Postgres, momo.JoinSQL(" and ", a, b))
	if err != nil {
		t.Fatal(err)
	}
	if want := `SELECT ((status = $1 OR kind = $2) AND (owner_id = $3)) FROM "documents"`; got.SQL != want {
		t.Errorf("and:\n got %s\nwant %s", got.SQL, want)
	}
	if !reflect.DeepEqual(got.Args, []any{"x", "y", int64(7)}) {
		t.Errorf("args = %v", got.Args)
	}

	got, err = joinRender(dialect.SQLite, momo.JoinSQL(",", momo.SQL("a"), momo.SQL("b + ?", 1)))
	if err != nil {
		t.Fatal(err)
	}
	if want := `SELECT a, b + ? FROM "documents"`; got.SQL != want || len(got.Args) != 1 {
		t.Errorf("comma: got %s %v, want %s", got.SQL, got.Args, want)
	}

	// Inside a larger statement the binds continue the outer numbering.
	q, err := momo.Select[unitTally](typedDB(dialect.Postgres)).From(momo.TableRef{Name: "documents"}).
		Columns(momo.CountAll()).
		Where(DocTenantID.Eq(9), momo.JoinSQL(" OR ", momo.SQL("a = ?", 1), momo.SQL("b = ?", 2))).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(q.SQL, `"documents"."tenant_id" = $1`) || !strings.Contains(q.SQL, `((a = $2) OR (b = $3))`) || len(q.Args) != 3 {
		t.Errorf("where: %s %v", q.SQL, q.Args)
	}

	got, err = joinRender(dialect.MySQL, momo.JoinSQL("||", momo.SQL("a"), momo.SQL("b")))
	if err != nil {
		t.Fatal(err)
	}
	if want := "SELECT CONCAT((a), (b)) FROM `documents`"; got.SQL != want {
		t.Errorf("mysql concat:\n got %s\nwant %s", got.SQL, want)
	}
	got, err = joinRender(dialect.Postgres, momo.JoinSQL("||", momo.SQL("a"), momo.SQL("b")))
	if err != nil || got.SQL != `SELECT ((a) || (b)) FROM "documents"` {
		t.Errorf("pg concat: %s, %v", got.SQL, err)
	}
	for sep, want := range map[string]string{" AND ": "1 = 1", " or ": "1 = 0"} {
		got, err = joinRender(dialect.Postgres, momo.JoinSQL(sep))
		if err != nil || got.SQL != `SELECT `+want+` FROM "documents"` {
			t.Errorf("empty %q join: %s, %v; want %s", sep, got.SQL, err, want)
		}
	}

	typedJoin := momo.JoinTypedSQL[int64]("+", momo.SQL("a"), momo.SQL("b"))
	if _, err := momo.Select[unitTally](typedDB(dialect.Postgres)).From(momo.TableRef{Name: "documents"}).
		Columns(typedJoin).SQL(); err != nil {
		t.Errorf("typed join: %v", err)
	}
}

// Catches: a separator carrying caller SQL, and degenerate joins rendering
// broken SQL instead of failing the build.
func TestJoinSQLValidation(t *testing.T) {
	a := momo.SQL("a")
	for name, e := range map[string]momo.Expr{
		"injection": momo.JoinSQL(" AND 1=1; DROP TABLE documents; -- ", a, a),
		"comment":   momo.JoinSQL("--", a, a),
		"word":      momo.JoinSQL("UNION", a, a),
		"empty":     momo.JoinSQL(","),
		"nil":       momo.JoinSQL(",", a, nil),
	} {
		if _, err := joinRender(dialect.Postgres, e); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: err = %v, want ErrValidation", name, err)
		}
	}
}
