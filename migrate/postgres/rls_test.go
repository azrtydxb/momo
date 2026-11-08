// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/azrtydxb/momo"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	"github.com/azrtydxb/momo/schema"
)

// countAs counts the rows of tenant_rows visible to tx, through raw SQL. It
// returns errors instead of failing the test so the surrounding transaction
// ends and the pool can close.
func countAs(tx *momo.Tx) (int, error) {
	rows, err := tx.DriverTx().Query(context.Background(), `SELECT count(*) FROM tenant_rows`, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return 0, fmt.Errorf("count returned no row: %w", rows.Err())
	}
	var n int
	if err := rows.Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// TestRowLevelSecurity: a policy applied by a migration confines a transaction
// scoped with Tx.SetLocal to its tenant, even through raw SQL, and the setting
// does not outlive the transaction on the pooled connection.
func TestRowLevelSecurity(t *testing.T) {
	db := testopen.Postgres(t)
	// The role's default privileges are installed when it is opened, so open
	// it before the migration creates the table.
	app := openAppRole(t)
	ctx := context.Background()
	const tenantIs = "tenant = current_setting('app.tenant', true)"
	desired := &schema.Schema{Dialect: "postgres", Tables: []schema.Table{{
		Name:       "tenant_rows",
		Columns:    []schema.Column{{Name: "id", Type: "bigint"}, {Name: "tenant", Type: "text"}},
		PrimaryKey: []string{"id"},
		RLS:        &schema.RLS{Enabled: true, Forced: true},
	}}}
	desired.Policies = []schema.Policy{{Table: "tenant_rows", Name: "tenant_only", Command: "ALL", Permissive: true, Using: tenantIs, Check: tenantIs}}

	mig, err := engineOn(t, db, migratetest.EmptyFS()).Diff(ctx, desired, "rls", migrate.DiffOptions{})
	if err != nil || mig == nil {
		t.Fatalf("diff: %v %v", mig, err)
	}
	if _, err := engineOn(t, db, migratetest.FS(mig.Files)).Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}

	// Rows are inserted per tenant, so the policy's CHECK passes whatever role
	// the administrator connection has.
	for id, tenant := range map[int]string{1: "a", 2: "b"} {
		err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			if err := tx.SetLocal(ctx, "app.tenant", tenant); err != nil {
				return err
			}
			_, err := tx.DriverTx().Exec(ctx, `INSERT INTO tenant_rows (id, tenant) VALUES ($1, $2)`, []any{id, tenant})
			return err
		})
		if err != nil {
			t.Fatalf("insert for tenant %s: %v", tenant, err)
		}
	}

	for tenant, want := range map[string]int{"a": 1, "b": 1, "nobody": 0} {
		var n int
		err := app.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			if err := tx.SetLocal(ctx, "app.tenant", tenant); err != nil {
				return err
			}
			var cerr error
			n, cerr = countAs(tx)
			return cerr
		})
		if err != nil || n != want {
			t.Fatalf("tenant %s must see %d row(s), got %d (%v)", tenant, want, n, err)
		}
	}

	// Without the setting the policy hides every row.
	err = app.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		n, cerr := countAs(tx)
		if cerr == nil && n != 0 {
			cerr = fmt.Errorf("a transaction without a tenant must see no rows, got %d", n)
		}
		return cerr
	})
	if err != nil {
		t.Fatal(err)
	}

	// The setting is transaction-scoped: the pooled connection must not carry it.
	var setting string
	mustQueryRow(t, pinned(t, app), `SELECT coalesce(current_setting('app.tenant', true), '')`, &setting)
	if setting != "" {
		t.Fatalf("set_config leaked past the transaction: %q", setting)
	}
}
