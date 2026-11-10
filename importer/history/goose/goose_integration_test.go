// SPDX-License-Identifier: Apache-2.0

//go:build integration

package goose

import (
	"context"
	"os"
	"testing"
	"testing/fstest"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/importtest"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestHistoryImporters(t *testing.T) {
	t.Run("goose", func(t *testing.T) {
		ctx := context.Background()
		live := testopen.Postgres(t)
		if _, err := momo.Exec(ctx, live, `CREATE TABLE goose_db_version (id serial PRIMARY KEY, version_id bigint NOT NULL, is_applied boolean NOT NULL, tstamp timestamp NULL DEFAULT now())`); err != nil {
			t.Fatal(err)
		}
		if _, err := momo.Exec(ctx, live, `INSERT INTO goose_db_version (version_id, is_applied) VALUES (0, true), (1, true), (2, true), (3, true)`); err != nil {
			t.Fatal(err)
		}
		pn, ok := live.Driver().(momo.Pinner)
		if !ok {
			t.Fatal("driver does not implement momo.Pinner")
		}
		conn, err := pn.Pin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = conn.Release() }()
		src, err := Read(os.DirFS("../../../internal/compat/goosefixture/migrations"), conn, "goose_db_version")
		if err != nil {
			t.Fatal(err)
		}
		if len(src.Applied) != 3 {
			t.Fatalf("goose version table not read: applied=%v", src.Applied)
		}
		if !src.Migrations[2].NoTx {
			t.Fatal("NO TRANSACTION annotation not read")
		}
		importtest.CheckImport(t, live, importtest.Input{Tool: "goose", Source: src, Exclude: []string{"goose_db_version"}})
	})
}

func TestReadRefusesDuplicateVersions(t *testing.T) {
	ctx := context.Background()
	live := testopen.Postgres(t)
	if _, err := momo.Exec(ctx, live, `CREATE TABLE goose_db_version (id serial PRIMARY KEY, version_id bigint NOT NULL, is_applied boolean NOT NULL, tstamp timestamp NULL DEFAULT now())`); err != nil {
		t.Fatal(err)
	}
	pn, ok := live.Driver().(momo.Pinner)
	if !ok {
		t.Fatal("driver does not implement momo.Pinner")
	}
	conn, err := pn.Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Release() }()
	fsys := fstest.MapFS{
		"00001_a.sql": {Data: []byte("-- +goose Up\nSELECT 1;\n")},
		"1_b.sql":     {Data: []byte("-- +goose Up\nSELECT 2;\n")},
	}
	if _, err := Read(fsys, conn, "goose_db_version"); err == nil {
		t.Fatal("00001 and 1 are the same goose version and must be refused")
	}
}
