// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres

import (
	"context"
	"slices"
	"testing"

	"github.com/azrtydxb/momo"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestInspectIndexesReportsInvalid(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	exec := func(sql string) {
		t.Helper()
		if _, err := db.Driver().Exec(ctx, sql, nil); err != nil {
			t.Fatalf("exec %q: %v", sql, err)
		}
	}
	exec(`CREATE TABLE inv (id bigint PRIMARY KEY, v int)`)
	exec(`INSERT INTO inv VALUES (1, 7), (2, 7)`)
	// A CONCURRENTLY build over duplicate data fails and leaves an invalid index.
	if _, err := db.Driver().Exec(ctx, `CREATE UNIQUE INDEX CONCURRENTLY inv_v_key ON inv (v)`, nil); err == nil {
		t.Fatal("expected the unique build to fail")
	}
	exec(`CREATE INDEX inv_ok ON inv (v)`)
	c, err := db.Driver().(momo.Pinner).Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Release() }()
	var oid uint32
	if err := queryOne(ctx, c, `SELECT to_regclass('inv')::oid`, nil, &oid); err != nil {
		t.Fatal(err)
	}
	ixs, invalid, err := Dialect{}.inspectIndexes(ctx, c, oid)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(invalid, []string{"inv_v_key"}) {
		t.Fatalf("invalid = %v", invalid)
	}
	if len(ixs) != 2 {
		t.Fatalf("indexes = %+v", ixs)
	}
}
