// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

func testRegistry() *momo.QueryRegistry {
	r := momo.NewQueryRegistry()
	r.Register("hooked_by_title", func(q momo.Querier) momo.Compilable {
		return momo.Select[hookedDoc](q).Where(hookedTitle.Eq("x"))
	})
	r.Register("a_raw", func(q momo.Querier) momo.Compilable {
		return momo.RawQuery(q, "SELECT id FROM t WHERE a = ? AND b = ?", 1, 2)
	})
	return r
}

// Catches: a registry that needs a database, loses a dialect's placeholder
// style, or returns the queries unsorted.
func TestQueryRegistryCompilesPerDialect(t *testing.T) {
	r := testRegistry()
	if got := r.Names(); len(got) != 2 || got[0] != "a_raw" || got[1] != "hooked_by_title" {
		t.Fatalf("Names = %v", got)
	}
	pg, err := r.Compile(dialect.Postgres)
	if err != nil || len(pg) != 2 {
		t.Fatalf("Compile(postgres) = %+v, %v", pg, err)
	}
	if pg[0].Name != "a_raw" || pg[0].SQL != "SELECT id FROM t WHERE a = $1 AND b = $2" {
		t.Errorf("raw on postgres = %+v", pg[0])
	}
	if !strings.Contains(pg[1].SQL, "$1") || !strings.Contains(pg[1].SQL, "hooked_docs") {
		t.Errorf("select on postgres = %q", pg[1].SQL)
	}
	my, err := r.Compile(dialect.MySQL)
	if err != nil {
		t.Fatal(err)
	}
	if my[0].SQL != "SELECT id FROM t WHERE a = ? AND b = ?" || strings.Contains(my[1].SQL, "$1") {
		t.Errorf("mysql SQL = %+v", my)
	}
}

// Catches: a duplicate name silently replacing the first query.
func TestQueryRegistryDuplicatePanics(t *testing.T) {
	r := testRegistry()
	defer func() {
		p := recover()
		msg, _ := p.(string)
		if !strings.Contains(msg, `"a_raw" is already registered`) {
			t.Fatalf("panic = %v, want a message naming the duplicate", p)
		}
	}()
	r.Register("a_raw", func(q momo.Querier) momo.Compilable { return momo.RawQuery(q, "SELECT 1") })
	t.Fatal("registering a name twice must panic")
}

func TestQueryRegistryRejectsBadRegistrations(t *testing.T) {
	for name, f := range map[string]func(){
		"empty name": func() { momo.NewQueryRegistry().Register("", func(momo.Querier) momo.Compilable { return nil }) },
		"nil build":  func() { momo.NewQueryRegistry().Register("x", nil) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s must panic", name)
				}
			}()
			f()
		}()
	}
}

// Catches: a failing builder being skipped instead of failing the listing.
func TestQueryRegistryCompileErrorNamesQuery(t *testing.T) {
	r := momo.NewQueryRegistry()
	r.Register("bad_args", func(q momo.Querier) momo.Compilable { return momo.RawQuery(q, "SELECT ?", 1, 2) })
	_, err := r.Compile(dialect.Postgres)
	if err == nil || !strings.Contains(err.Error(), `"bad_args"`) {
		t.Fatalf("Compile = %v, want an error naming bad_args", err)
	}
	r2 := momo.NewQueryRegistry()
	r2.Register("nil_build", func(momo.Querier) momo.Compilable { return nil })
	if _, err := r2.Compile(dialect.Postgres); err == nil {
		t.Fatal("a nil Compilable must fail")
	}
}

// Catches: RegisterQuery not reaching the process-wide registry.
func TestRegisterQueryUsesDefaultRegistry(t *testing.T) {
	momo.RegisterQuery("registry_test_default", func(q momo.Querier) momo.Compilable { return momo.RawQuery(q, "SELECT 1") })
	found := false
	for _, n := range momo.DefaultQueries().Names() {
		found = found || n == "registry_test_default"
	}
	if !found {
		t.Fatal("RegisterQuery did not register in DefaultQueries")
	}
}

// Catches: the offline handle reaching for a database.
func TestOfflineNeverExecutes(t *testing.T) {
	db := momo.NewOffline(dialect.SQLite)
	if _, err := momo.Exec(t.Context(), db, "SELECT 1"); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("Exec on an offline handle = %v, want ErrUnsupported", err)
	}
}
