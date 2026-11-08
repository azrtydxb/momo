// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestTxSetupPanicDoesNotLeakTransaction(t *testing.T) {
	db, pool := rlsDB(t, momo.WithImplicitTx(), momo.WithTxSetup(func(context.Context, *momo.Tx) error {
		panic("setup exploded")
	}))
	for i := 0; i < 8; i++ {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("the setup panic was swallowed")
				}
			}()
			_, _ = momo.Select[suite.Document](db).Count(context.Background())
		}()
	}
	assertNoLeakedConns(t, pool)
}

func TestTxSetupFailsClosedOnPinnedConn(t *testing.T) {
	db, pool := rlsDB(t, momo.WithTxSetup(tenantSetup))
	ctx := withTenant(context.Background(), 1)
	pinner, ok := db.Driver().(momo.Pinner)
	if !ok {
		t.Fatal("the pgx driver must support Pin")
	}
	pc, err := pinner.Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pinned := db.OnConn(pc)
	if _, err := momo.Select[suite.Document](pinned).Count(ctx); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("Count on a pinned handle = %v, want ErrUnsupported", err)
	}
	if _, err := momo.Exec(ctx, pinned, "DELETE FROM documents"); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("Exec on a pinned handle = %v, want ErrUnsupported", err)
	}
	if _, ok := momo.CopierOf(pinned); ok {
		t.Error("CopierOf on a pinned handle with a setup must report none")
	}
	if _, ok := momo.BatcherOf(pinned); ok {
		t.Error("BatcherOf on a pinned handle with a setup must report none")
	}
	if err := pc.Release(); err != nil {
		t.Fatal(err)
	}
	assertNoLeakedConns(t, pool)
}

func TestImplicitTxReturningExistsAndCancel(t *testing.T) {
	db, pool := rlsDB(t, momo.WithTxSetup(tenantSetup), momo.WithImplicitTx(), momo.WithMaxParams(20))
	ctx := context.Background()
	t1, t2 := withTenant(ctx, 1), withTenant(ctx, 2)
	if _, err := momo.Insert[suite.Document](db).Models(docs(1, 4, "a")).Exec(t1); err != nil {
		t.Fatal(err)
	}
	if _, err := momo.Insert[suite.Document](db).Models(docs(2, 2, "b")).Exec(t2); err != nil {
		t.Fatal(err)
	}

	if ok, err := momo.Select[suite.Document](db).Exists(ctx); err != nil || ok {
		t.Fatalf("unscoped Exists = %v, %v; want false", ok, err)
	}
	if ok, err := momo.Select[suite.Document](db).Exists(t2); err != nil || !ok {
		t.Fatalf("tenant 2 Exists = %v, %v; want true", ok, err)
	}

	// INSERT ... RETURNING and UPDATE ... RETURNING commit on exhaustion.
	ins, err := momo.Insert[suite.Document](db).Model(&suite.Document{TenantID: 1, Title: "r"}).ReturningAll().Rows(t1)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for ins.Next() {
		n++
	}
	if err := ins.Err(); err != nil || n != 1 {
		t.Fatalf("insert returning n=%d err=%v", n, err)
	}
	if err := ins.Close(); err != nil {
		t.Fatal(err)
	}
	if c, err := momo.Select[suite.Document](db).Count(t1); err != nil || c != 5 {
		t.Fatalf("after INSERT RETURNING count = %d, %v; want 5", c, err)
	}
	upd, err := momo.Update[suite.Document](db).Set(suite.DocTitle.Set("z")).Where(suite.DocTenantID.Eq(2)).ReturningAll().Rows(t2)
	if err != nil {
		t.Fatal(err)
	}
	n = 0
	for upd.Next() {
		n++
	}
	if err := upd.Err(); err != nil || n != 2 {
		t.Fatalf("update returning n=%d err=%v; want 2 (tenant 2 only)", n, err)
	}
	_ = upd.Close()
	if c, err := momo.Select[suite.Document](db).Where(suite.DocTitle.Eq("z")).Count(t2); err != nil || c != 2 {
		t.Fatalf("committed update count = %d, %v; want 2", c, err)
	}

	// A write abandoned by a cancelled context is rolled back, and Close
	// reports it instead of silently discarding the write.
	cctx, cancel := context.WithCancel(t1)
	w, err := momo.Update[suite.Document](db).Set(suite.DocTitle.Set("lost")).Where(suite.DocTenantID.Eq(1)).ReturningAll().Rows(cctx)
	if err != nil {
		t.Fatal(err)
	}
	if !w.Next() {
		t.Fatal("no row returned")
	}
	cancel()
	if err := w.Close(); err == nil {
		t.Fatal("Close after cancel returned nil: the discarded write went unreported")
	}
	if c, err := momo.Select[suite.Document](db).Where(suite.DocTitle.Eq("lost")).Count(t1); err != nil || c != 0 {
		t.Fatalf("cancelled write persisted: count = %d, %v", c, err)
	}

	// Cancelling mid-stream ends Each with the cancellation and frees the connection.
	sctx, scancel := context.WithCancel(t1)
	err = momo.Select[suite.Document](db).Each(sctx, func(*suite.Document) error {
		scancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Each after cancel = %v, want context.Canceled", err)
	}

	// A panic in a streaming callback rolls back, releases the connection and propagates.
	func() {
		defer func() {
			if recover() == nil {
				t.Error("callback panic was swallowed")
			}
		}()
		_ = momo.Select[suite.Document](db).Each(t1, func(*suite.Document) error { panic("cb") })
	}()
	assertNoLeakedConns(t, pool)
}

func TestImplicitTxRLSRelationLoads(t *testing.T) {
	appURL := testdb.PostgresAppRoleURL(t)
	admin := testopen.Postgres(t)
	relfixture.CreateSchema(t, admin)
	relfixture.SeedBasic(t, admin)
	for _, stmt := range []string{
		"ALTER TABLE comments ENABLE ROW LEVEL SECURITY",
		"ALTER TABLE comments FORCE ROW LEVEL SECURITY",
		`CREATE POLICY doc_scope ON comments
			USING (document_id = nullif(current_setting('app.tenant_id', true), '')::bigint)`,
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
	db := momo.New(pgxdriver.New(pool), momo.WithTxSetup(tenantSetup), momo.WithImplicitTx())
	ctx := context.Background()
	scoped := withTenant(ctx, 1) // the policy lets it see the comments of document 1 only

	count := func(c context.Context, ds []models.Document) string {
		t.Helper()
		got, err := momo.LoadCounts(c, db, ds, document.Comments)
		if err != nil {
			t.Fatal(err)
		}
		return fmt.Sprint(got)
	}
	ds := []models.Document{{ID: 1}, {ID: 3}}
	if got := count(ctx, ds); got != "[0 0]" {
		t.Fatalf("unscoped LoadCounts = %s, want [0 0]", got)
	}
	if got := count(scoped, ds); got != "[3 0]" {
		t.Fatalf("scoped LoadCounts = %s, want [3 0]", got)
	}
	if err := momo.Load(scoped, db, ds, document.Comments); err != nil {
		t.Fatal(err)
	}
	if len(ds[0].Comments) != 3 || len(ds[1].Comments) != 0 {
		t.Fatalf("scoped Load: %d and %d comments, want 3 and 0", len(ds[0].Comments), len(ds[1].Comments))
	}
	pre, err := momo.Select[models.Document](db).Preload(document.Comments).OrderBy(document.ID.Asc()).All(scoped)
	if err != nil {
		t.Fatal(err)
	}
	if len(pre) != 4 || len(pre[0].Comments) != 3 || len(pre[2].Comments) != 0 {
		t.Fatalf("scoped Preload: %d documents, comments %d and %d", len(pre), len(pre[0].Comments), len(pre[2].Comments))
	}
	pre, err = momo.Select[models.Document](db).Preload(document.Comments).All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range pre {
		if len(d.Comments) != 0 {
			t.Fatalf("unscoped Preload leaked %d comments", len(d.Comments))
		}
	}
	assertNoLeakedConns(t, pool)
}
