// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

type tenantKey struct{}

func withTenant(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, tenantKey{}, id)
}

// tenantSetup is the fail-closed pattern: it sets app.tenant_id from the
// context, and sets nothing when the context carries no tenant.
func tenantSetup(ctx context.Context, tx *momo.Tx) error {
	if id, ok := ctx.Value(tenantKey{}).(int64); ok {
		return momo.SetConfigLocal(ctx, tx, "app.tenant_id", strconv.FormatInt(id, 10))
	}
	return nil
}

// rlsDB creates the documents table under a row-level-security policy owned by
// the administrator and returns a *DB that connects as the non-superuser
// application role (so RLS applies) with opts, plus its pool.
func rlsDB(t *testing.T, opts ...momo.Option) (*momo.DB, *pgxpool.Pool) {
	t.Helper()
	// The app role URL must exist before the tables so that the schema's
	// default privileges cover them.
	appURL := testdb.PostgresAppRoleURL(t)
	admin := testopen.Postgres(t)
	suite.CreateDocuments(t, admin, suite.CapsOf(admin))
	for _, stmt := range []string{
		"ALTER TABLE documents ENABLE ROW LEVEL SECURITY",
		"ALTER TABLE documents FORCE ROW LEVEL SECURITY",
		`CREATE POLICY tenant_isolation ON documents
			USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::bigint)
			WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::bigint)`,
	} {
		mustExecPG(t, admin, stmt)
	}
	cfg, err := pgxpool.ParseConfig(appURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 3
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return momo.New(pgxdriver.New(pool), opts...), pool
}

func assertNoLeakedConns(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if n := pool.Stat().AcquiredConns(); n != 0 {
		t.Fatalf("%d connections still acquired, an implicit transaction leaked its connection", n)
	}
}

func docs(tenant int64, n int, prefix string) []suite.Document {
	out := make([]suite.Document, n)
	for i := range out {
		out[i] = suite.Document{TenantID: tenant, Title: fmt.Sprintf("%s-%d", prefix, i)}
	}
	return out
}

func TestImplicitTxRLSFailClosed(t *testing.T) {
	// A low parameter limit forces the bulk insert into several chunks, each
	// of which momo runs in a transaction it opens itself.
	db, pool := rlsDB(t, momo.WithTxSetup(tenantSetup), momo.WithImplicitTx(), momo.WithMaxParams(20))
	ctx := context.Background()
	t1, t2 := withTenant(ctx, 1), withTenant(ctx, 2)

	// Internal chunked insert transactions get the setup: WITH CHECK would
	// reject any chunk that ran without it.
	if _, err := momo.Insert[suite.Document](db).Models(docs(1, 7, "a")).Exec(t1); err != nil {
		t.Fatalf("chunked insert as tenant 1: %v", err)
	}
	if _, err := momo.Insert[suite.Document](db).Models(docs(2, 3, "b")).Exec(t2); err != nil {
		t.Fatalf("chunked insert as tenant 2: %v", err)
	}
	// A forgotten scope fails closed on writes too.
	if _, err := momo.Insert[suite.Document](db).Models(docs(1, 1, "x")).Exec(ctx); err == nil {
		t.Fatal("insert without a tenant scope succeeded")
	}
	// A scope for another tenant cannot write rows of this one.
	if _, err := momo.Insert[suite.Document](db).Models(docs(1, 1, "x")).Exec(t2); err == nil {
		t.Fatal("tenant 2 inserted a tenant 1 row")
	}

	// Auto-commit reads.
	if n, err := momo.Select[suite.Document](db).Count(ctx); err != nil || n != 0 {
		t.Fatalf("unscoped count = %d, %v; want 0 (fail closed)", n, err)
	}
	if rows, err := momo.Select[suite.Document](db).All(ctx); err != nil || len(rows) != 0 {
		t.Fatalf("unscoped All = %d rows, %v; want 0", len(rows), err)
	}
	if n, err := momo.Select[suite.Document](db).Count(t1); err != nil || n != 7 {
		t.Fatalf("tenant 1 count = %d, %v; want 7", n, err)
	}
	if n, err := momo.Select[suite.Document](db).Count(t2); err != nil || n != 3 {
		t.Fatalf("tenant 2 count = %d, %v; want 3", n, err)
	}
	if got, err := momo.Scalars[int64](t2, db, "SELECT count(*) FROM documents"); err != nil || len(got) != 1 || got[0] != 3 {
		t.Fatalf("raw scalars as tenant 2 = %v, %v; want [3]", got, err)
	}

	// Auto-commit writes only touch the tenant's rows.
	n, err := momo.Exec(t2, db, "UPDATE documents SET secret = ?", "s")
	if err != nil || n != 3 {
		t.Fatalf("update as tenant 2 affected %d, %v; want 3", n, err)
	}
	if n, err := momo.Exec(ctx, db, "UPDATE documents SET secret = ?", "s"); err != nil || n != 0 {
		t.Fatalf("unscoped update affected %d, %v; want 0", n, err)
	}

	// Streaming: Each and Iter see only the tenant's rows, and commit when
	// exhausted.
	seen := 0
	if err := momo.Select[suite.Document](db).Each(t1, func(d *suite.Document) error {
		if d.TenantID != 1 {
			t.Errorf("tenant 1 stream returned a tenant %d row", d.TenantID)
		}
		seen++
		return nil
	}); err != nil || seen != 7 {
		t.Fatalf("Each saw %d rows, %v; want 7", seen, err)
	}
	assertNoLeakedConns(t, pool)
	seen = 0
	for _, err := range momo.Select[suite.Document](db).Iter(ctx) {
		if err != nil {
			t.Fatal(err)
		}
		seen++
	}
	if seen != 0 {
		t.Fatalf("unscoped Iter returned %d rows, want 0", seen)
	}
	assertNoLeakedConns(t, pool)

	// Breaking out of Iter closes the rows and ends the transaction. More
	// iterations than pool connections would block if one leaked.
	for i := 0; i < 8; i++ {
		for _, err := range momo.Select[suite.Document](db).Iter(t1) {
			if err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	assertNoLeakedConns(t, pool)

	// An error from the Each callback ends the stream without leaking.
	boom := errors.New("boom")
	for i := 0; i < 8; i++ {
		err := momo.Select[suite.Document](db).Each(t1, func(*suite.Document) error { return boom })
		if !errors.Is(err, boom) {
			t.Fatalf("Each error = %v, want boom", err)
		}
	}
	assertNoLeakedConns(t, pool)

	// A failing statement rolls back and releases its connection.
	for i := 0; i < 8; i++ {
		if _, err := momo.Exec(t1, db, "SELECT * FROM no_such_table"); err == nil {
			t.Fatal("expected an error")
		}
	}
	assertNoLeakedConns(t, pool)
}

func TestTxSetupWithoutImplicitTxLeavesAutoCommitUnscoped(t *testing.T) {
	db, pool := rlsDB(t, momo.WithTxSetup(tenantSetup), momo.WithMaxParams(20))
	ctx := context.Background()
	t1 := withTenant(ctx, 1)
	// Explicit and internal transactions are scoped.
	if _, err := momo.Insert[suite.Document](db).Models(docs(1, 5, "a")).Exec(t1); err != nil {
		t.Fatalf("chunked insert as tenant 1: %v", err)
	}
	err := db.Tx(t1, momo.TxOptions{}, func(tx *momo.Tx) error {
		n, err := momo.Select[suite.Document](tx).Count(t1)
		if err != nil || n != 5 {
			return fmt.Errorf("in Tx count = %d, %v; want 5", n, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// An auto-commit read is not: it matches nothing even with a scope.
	if n, err := momo.Select[suite.Document](db).Count(t1); err != nil || n != 0 {
		t.Fatalf("auto-commit count = %d, %v; want 0 without WithImplicitTx", n, err)
	}
	assertNoLeakedConns(t, pool)
}

func TestTxSetupRunsOncePerTransactionNotPerSavepoint(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		var mu sync.Mutex
		calls := 0
		counted := momo.New(db.Driver(), momo.WithTxSetup(func(context.Context, *momo.Tx) error {
			mu.Lock()
			defer mu.Unlock()
			calls++
			return nil
		}))
		ctx := context.Background()
		err := counted.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			return tx.Tx(ctx, func(inner *momo.Tx) error {
				return inner.Tx(ctx, func(*momo.Tx) error { return nil })
			})
		})
		if err != nil {
			t.Fatal(err)
		}
		if calls != 1 {
			t.Fatalf("setup ran %d times for one transaction with two savepoints, want 1", calls)
		}
	})
}

func TestTxSetupErrorRollsBack(t *testing.T) {
	boom := errors.New("no tenant")
	db, pool := rlsDB(t, momo.WithImplicitTx(), momo.WithTxSetup(func(ctx context.Context, tx *momo.Tx) error {
		if _, ok := ctx.Value(tenantKey{}).(int64); !ok {
			return boom
		}
		return tenantSetup(ctx, tx)
	}))
	ctx := context.Background()
	for i := 0; i < 8; i++ {
		if _, err := momo.Select[suite.Document](db).Count(ctx); !errors.Is(err, boom) {
			t.Fatalf("Count error = %v, want the setup error", err)
		}
		if err := db.Tx(ctx, momo.TxOptions{}, func(*momo.Tx) error {
			t.Error("Tx callback ran after a failed setup")
			return nil
		}); !errors.Is(err, boom) {
			t.Fatalf("Tx error = %v, want the setup error", err)
		}
		if _, err := momo.Insert[suite.Document](db).Models(docs(1, 5, "a")).Exec(ctx); !errors.Is(err, boom) {
			t.Fatalf("Insert error = %v, want the setup error", err)
		}
	}
	assertNoLeakedConns(t, pool)
	if n, err := momo.Select[suite.Document](db).Count(withTenant(ctx, 1)); err != nil || n != 0 {
		t.Fatalf("count = %d, %v; failed setups must leave no rows", n, err)
	}
}

// TestImplicitTxPgBouncer runs the tenant scope through PgBouncer in
// transaction mode with many concurrent tenants. The setting is transaction
// local, so each statement sees exactly its own tenant and an unscoped
// statement never sees a value left behind on a shared backend.
func TestImplicitTxPgBouncer(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxdriver.NewPool(ctx, testdb.PgBouncerURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	db := momo.New(pgxdriver.New(pool), momo.WithTxSetup(tenantSetup), momo.WithImplicitTx())

	const tenants, rounds = 6, 25
	var wg sync.WaitGroup
	errs := make(chan error, tenants*rounds*2)
	for tn := int64(1); tn <= tenants; tn++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < rounds; i++ {
				got, err := momo.Scalars[string](withTenant(ctx, tn), db, "SELECT current_setting('app.tenant_id', true)")
				if err != nil || len(got) != 1 || got[0] != strconv.FormatInt(tn, 10) {
					errs <- fmt.Errorf("tenant %d saw %v, %v", tn, got, err)
				}
				got, err = momo.Scalars[string](ctx, db, "SELECT coalesce(current_setting('app.tenant_id', true), '')")
				if err != nil || len(got) != 1 || got[0] != "" {
					errs <- fmt.Errorf("unscoped statement saw %v, %v; the setting leaked", got, err)
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
