// SPDX-License-Identifier: Apache-2.0

//go:build integration && cgo && !windows

package sqlitedriver

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/azrtydxb/momo/types"
)

// TestScanTimeDestinations covers every time-shaped destination a model can
// use: the text column holds a time, and none of them may need a declared
// TIMESTAMP type.
func TestScanTimeDestinations(t *testing.T) {
	_, drv, err := Open(testdb.SQLitePath(t), Config{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := drv.Exec(ctx, "CREATE TABLE ts (at text, nul text, day text)", nil); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 7, 20, 10, 12, 123000000, time.UTC)
	if _, err := drv.Exec(ctx, "INSERT INTO ts VALUES (?, NULL, '2026-11-07')", []any{at}); err != nil {
		t.Fatal(err)
	}
	rows, err := drv.Query(ctx, "SELECT at, at, at, at, nul, nul, nul, day FROM ts", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatalf("no row: %v", rows.Err())
	}
	var (
		plain           time.Time
		ptr             *time.Time
		nullTime        sql.NullTime
		nullGeneric     sql.Null[time.Time]
		nilPtr          *time.Time
		nullTimeNull    sql.NullTime
		nullGenericNull sql.Null[time.Time]
		date            types.Date
	)
	if err := rows.Scan(&plain, &ptr, &nullTime, &nullGeneric, &nilPtr, &nullTimeNull, &nullGenericNull, &date); err != nil {
		t.Fatal(err)
	}
	if !plain.Equal(at) || ptr == nil || !ptr.Equal(at) || !nullTime.Valid || !nullTime.Time.Equal(at) || !nullGeneric.Valid || !nullGeneric.V.Equal(at) {
		t.Fatalf("time destinations = %v %v %+v %+v", plain, ptr, nullTime, nullGeneric)
	}
	if nilPtr != nil || nullTimeNull.Valid || nullGenericNull.Valid {
		t.Fatalf("NULL destinations = %v %+v %+v", nilPtr, nullTimeNull, nullGenericNull)
	}
	if date.Time.Year() != 2026 || date.Time.Month() != 11 || date.Time.Day() != 7 {
		t.Fatalf("types.Date = %v", date.Time)
	}
}

// TestRowsReportColumns: MapRows and other helpers need momo.ColumnNamer, on
// pooled, transaction and pinned rows alike.
func TestRowsReportColumns(t *testing.T) {
	db, drv, err := Open(testdb.SQLitePath(t), Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	check := func(name string, rows momo.Rows, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		defer rows.Close()
		cn, ok := rows.(momo.ColumnNamer)
		if !ok {
			t.Fatalf("%s: rows do not implement ColumnNamer", name)
		}
		cols, err := cn.Columns()
		if err != nil || len(cols) != 2 || cols[0] != "a" || cols[1] != "b" {
			t.Fatalf("%s: columns = %v, %v", name, cols, err)
		}
	}
	rows, err := drv.Query(ctx, "SELECT 1 AS a, 2 AS b", nil)
	check("pool", rows, err)
	tx, err := drv.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	rows, err = tx.Query(ctx, "SELECT 1 AS a, 2 AS b", nil)
	check("tx", rows, err)
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	ro, err := drv.Begin(ctx, momo.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	rows, err = ro.Query(ctx, "SELECT 1 AS a, 2 AS b", nil)
	check("read-only tx", rows, err)
	if err := ro.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := drv.(momo.Pinner).Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rows, err = p.Query(ctx, "SELECT 1 AS a, 2 AS b", nil)
	check("pinned", rows, err)
	if err := p.Release(); err != nil {
		t.Fatal(err)
	}
}

// TestScanErrorHidesValue: a conversion failure must not render the stored
// text, which may be user data; the cause stays reachable.
func TestScanErrorHidesValue(t *testing.T) {
	_, drv, err := Open(testdb.SQLitePath(t), Config{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	rows, err := drv.Query(ctx, "SELECT 'secret-user-value'", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	rows.Next()
	var n int64
	err = rows.Scan(&n)
	if err == nil {
		t.Fatal("scanning text into an int64 succeeded")
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatalf("the error renders the stored value: %v", err)
	}
	if u, ok := err.(interface{ Unwrap() error }); !ok || u.Unwrap() == nil {
		t.Fatalf("the cause is not reachable: %T", err)
	}
	// The pool has one connection: release it before the next query.
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	var tm time.Time
	rows2, err := drv.Query(ctx, "SELECT 'secret-user-value'", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rows2.Close()
	rows2.Next()
	if err := rows2.Scan(&tm); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("time scan error = %v", err)
	}
}

// TestSQLiteVecRoundTrip: Capabilities.Vectors is true, so sqlite-vec must
// work through the driver on every connection.
func TestSQLiteVecRoundTrip(t *testing.T) {
	db, drv, err := Open(testdb.SQLitePath(t), Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	if _, err := drv.Exec(ctx, "CREATE TABLE emb (id integer primary key, v blob)", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := drv.Exec(ctx, "INSERT INTO emb VALUES (1, vec_f32(?)), (2, vec_f32(?))", []any{"[1,2,3]", "[0,0,0]"}); err != nil {
		t.Fatal(err)
	}
	rows, err := drv.Query(ctx, "SELECT vec_to_json(v), vec_distance_l2(v, vec_f32(?)) FROM emb ORDER BY vec_distance_l2(v, vec_f32(?)), id", []any{"[0,0,0]", "[0,0,0]"})
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []string
	var dists []float64
	for rows.Next() {
		var j string
		var d float64
		if err := rows.Scan(&j, &d); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, strings.ReplaceAll(j, " ", ""))
		dists = append(dists, d)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || !strings.HasPrefix(ids[0], "[0") || !strings.HasPrefix(ids[1], "[1") || dists[0] != 0 || dists[1] < 3.7 || dists[1] > 3.8 {
		t.Fatalf("vectors = %v, distances = %v", ids, dists)
	}
}

// TestSQLiteConfigOnEveryConnection checks the PRAGMAs on two physical
// connections, not only the first one the pool opens.
func TestSQLiteConfigOnEveryConnection(t *testing.T) {
	db, _, err := Open(testdb.SQLitePath(t), Config{MaxOpenConns: 2})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	c1, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer c1.Close()
	c2, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()
	for i, c := range []*sql.Conn{c1, c2} {
		var mode string
		var syncMode, fk, busy int
		if err := c.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
			t.Fatal(err)
		}
		if err := c.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&syncMode); err != nil {
			t.Fatal(err)
		}
		if err := c.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
			t.Fatal(err)
		}
		if err := c.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busy); err != nil {
			t.Fatal(err)
		}
		if mode != "wal" || syncMode != 1 || fk != 1 || busy != 5000 {
			t.Fatalf("connection %d: journal=%s synchronous=%d foreign_keys=%d busy_timeout=%d", i, mode, syncMode, fk, busy)
		}
	}
}
