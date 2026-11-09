// SPDX-License-Identifier: Apache-2.0

package vecsearch_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/vecsearch"
)

func TestApplyBindsSetConfig(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv)
	tx, err := db.Begin(t.Context(), momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := vecsearch.Apply(t.Context(), tx, vecsearch.Settings{HNSWEfSearch: 100}); err != nil {
		t.Fatal(err)
	}
	calls := drv.Calls()
	stmt := calls[len(calls)-1]
	if stmt.SQL != "SELECT set_config($1, $2, true)" {
		t.Fatalf("SQL = %q, want a bound set_config call", stmt.SQL)
	}
	if !slices.Equal(stmt.Args, []any{"hnsw.ef_search", "100"}) {
		t.Fatalf("args = %v, want the name and value bound as parameters", stmt.Args)
	}
}

func TestSearchSQLiteSupport(t *testing.T) {
	drv := fakedriver.New(dialect.SQLite)
	db := momo.New(drv)
	tx, err := db.Begin(t.Context(), momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := vecsearch.Apply(t.Context(), tx, vecsearch.Settings{ExactSearch: true}); err != nil {
		t.Fatalf("ExactSearch alone on SQLite: %v, want no-op", err)
	}
	if err := vecsearch.Apply(t.Context(), tx, vecsearch.Settings{HNSWEfSearch: 10}); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
	if n := len(drv.Calls()); n != 0 {
		t.Fatalf("SQLite issued %d statements, want none", n)
	}
}

func TestSearchIterativeNeedsPgvector08(t *testing.T) {
	for _, tc := range []struct {
		version string
		ok      bool
	}{{"0.7.4", false}, {"0.8.0", true}} {
		drv := fakedriver.New(dialect.Postgres)
		drv.SetRows([][]any{{tc.version}})
		db := momo.New(drv)
		tx, err := db.Begin(t.Context(), momo.TxOptions{})
		if err != nil {
			t.Fatal(err)
		}
		err = vecsearch.Apply(t.Context(), tx, vecsearch.Settings{HNSWIterativeScan: vecsearch.IterativeRelaxedOrder})
		if tc.ok && err != nil {
			t.Fatalf("%s: %v", tc.version, err)
		}
		if !tc.ok {
			if !errors.Is(err, momo.ErrUnsupported) {
				t.Fatalf("%s: err = %v, want ErrUnsupported", tc.version, err)
			}
			for _, c := range drv.Calls() {
				if strings.Contains(c.SQL, "set_config") {
					t.Fatalf("set_config issued despite unsupported version: %q", c.SQL)
				}
			}
		}
	}
}

func TestSearchScanMemMultiplierNeedsPgvector08(t *testing.T) {
	for _, tc := range []struct {
		version string
		ok      bool
	}{{"0.7.4", false}, {"0.8.0", true}} {
		drv := fakedriver.New(dialect.Postgres)
		drv.SetRows([][]any{{tc.version}})
		db := momo.New(drv)
		tx, err := db.Begin(t.Context(), momo.TxOptions{})
		if err != nil {
			t.Fatal(err)
		}
		err = vecsearch.Apply(t.Context(), tx, vecsearch.Settings{HNSWScanMemMultiplier: 2})
		if tc.ok {
			if err != nil {
				t.Fatalf("%s: %v", tc.version, err)
			}
			found := false
			for _, c := range drv.Calls() {
				if strings.Contains(c.SQL, "set_config") && slices.Equal(c.Args, []any{"hnsw.scan_mem_multiplier", "2"}) {
					found = true
				}
			}
			if !found {
				t.Fatalf("%s: no set_config(hnsw.scan_mem_multiplier, 2) among %v", tc.version, drv.Calls())
			}
		}
		if !tc.ok {
			if !errors.Is(err, momo.ErrUnsupported) {
				t.Fatalf("%s: err = %v, want ErrUnsupported", tc.version, err)
			}
			for _, c := range drv.Calls() {
				if strings.Contains(c.SQL, "set_config") {
					t.Fatalf("set_config issued despite unsupported version: %q", c.SQL)
				}
			}
		}
	}
}

func TestSearchNoVersionQueryWithoutIterativeFields(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv)
	tx, err := db.Begin(t.Context(), momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := vecsearch.Apply(t.Context(), tx, vecsearch.Settings{HNSWEfSearch: 50, IVFFlatProbes: 3, ExactSearch: true}); err != nil {
		t.Fatal(err)
	}
	for _, c := range drv.Calls() {
		if strings.Contains(c.SQL, "pg_extension") {
			t.Fatalf("version queried needlessly: %q", c.SQL)
		}
	}
}

func TestSearchMySQLExactSearchUnsupported(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.MySQL))
	tx, err := db.Begin(t.Context(), momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := vecsearch.Apply(t.Context(), tx, vecsearch.Settings{ExactSearch: true}); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
}

func TestWithAppliesSettingsAndRunsCallback(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv)
	ran := false
	err := vecsearch.With(t.Context(), db, vecsearch.Settings{HNSWEfSearch: 77}, func(tx *momo.Tx) error {
		ran = true
		return nil
	})
	if err != nil || !ran {
		t.Fatalf("With = %v, ran = %v, want nil and true", err, ran)
	}
	found := false
	for _, c := range drv.Calls() {
		if strings.Contains(c.SQL, "set_config") && slices.Equal(c.Args, []any{"hnsw.ef_search", "77"}) {
			found = true
		}
	}
	if !found {
		t.Fatalf("no set_config(hnsw.ef_search, 77) among %v", drv.Calls())
	}
	if drv.Commits() != 1 {
		t.Fatalf("commits = %d, want 1", drv.Commits())
	}
}

func TestWithRejectsInvalidSettingsBeforeBegin(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	err := vecsearch.With(t.Context(), momo.New(drv), vecsearch.Settings{HNSWEfSearch: 5000}, func(*momo.Tx) error {
		t.Fatal("callback ran")
		return nil
	})
	if !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
	if n := len(drv.Calls()); n != 0 {
		t.Fatalf("invalid settings issued %d statements, want none", n)
	}
}
