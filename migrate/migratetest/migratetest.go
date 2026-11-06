// SPDX-License-Identifier: Apache-2.0

// Package migratetest holds helpers shared by the migrate engine tests. The
// engine tests run against the real dialect, migrate/postgres.New(), with its
// own lock, session setup and history table.
package migratetest

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// WithSum returns a copy of fsys with a momo.sum that matches its files.
func WithSum(t testing.TB, fsys fstest.MapFS) fs.FS {
	t.Helper()
	out := fstest.MapFS{}
	for k, v := range fsys {
		out[k] = v
	}
	delete(out, migrate.SumFileName)
	sum, err := migrate.ComputeSum(out)
	if err != nil {
		t.Fatalf("migratetest: compute sum: %v", err)
	}
	var buf bytes.Buffer
	if err := sum.Write(&buf); err != nil {
		t.Fatalf("migratetest: write sum: %v", err)
	}
	out[migrate.SumFileName] = &fstest.MapFile{Data: buf.Bytes()}
	return out
}

// NewEngine builds an engine on d, drv and fsys, failing the test on error. It
// passes migrate.WithoutSum, so a test directory needs no momo.sum; a test of the
// sum checks builds the engine with migrate.NewEngine.
func NewEngine(t testing.TB, d migrate.Dialect, drv momo.Driver, fsys fs.FS, opts ...migrate.Option) *migrate.Engine {
	t.Helper()
	e, err := migrate.NewEngine(drv, fsys, append([]migrate.Option{migrate.WithDialect(d), migrate.WithoutSum()}, opts...)...)
	if err != nil {
		t.Fatalf("migratetest: new engine: %v", err)
	}
	return e
}

// EmptyFS is a migration directory without files.
func EmptyFS() fs.FS { return fstest.MapFS{} }

// FS turns the file map of a generated migration (relative name to content)
// into a migration directory.
func FS(files map[string][]byte) fs.FS {
	out := fstest.MapFS{}
	for name, data := range files {
		out[name] = &fstest.MapFile{Data: data}
	}
	return out
}

// Diff prints the difference between two schemas as their normalized JSON, for
// a failing schema.Equal.
func Diff(got, want *schema.Schema) string {
	x, _ := json.MarshalIndent(schema.Normalized(got), "", " ")
	y, _ := json.MarshalIndent(schema.Normalized(want), "", " ")
	return "got:\n" + string(x) + "\nwant:\n" + string(y)
}
