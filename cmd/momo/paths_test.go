// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/migrate"
)

// TestWindowsPaths asserts generated imports and migration paths use forward slashes.
func TestWindowsPaths(t *testing.T) {
	t.Run("generated import path", func(t *testing.T) {
		root := t.TempDir()
		// filepath.Join uses backslashes on Windows, so this genuinely tests the conversion to slashes.
		got, err := gen.ImportPath("example.com/app", root, filepath.Join(root, "internal", "models", "user"))
		if err != nil {
			t.Fatal(err)
		}
		if want := "example.com/app/internal/models/user"; got != want {
			t.Fatalf("ImportPath = %q, want %q", got, want)
		}
	})

	t.Run("migration and momo.sum paths", func(t *testing.T) {
		dir := t.TempDir()
		nested := filepath.Join(dir, "archive", "2026") // built with filepath.Join: backslashes on Windows
		if err := os.MkdirAll(nested, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(nested, "20260101000000_init.up.sql"), []byte("CREATE TABLE t (id int);\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		sum, err := migrate.ComputeSum(os.DirFS(dir))
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := sum.Files["archive/2026/20260101000000_init.up.sql"]; !ok {
			t.Fatalf("momo.sum entry must use a forward slash, got %v", sum.Files)
		}
		for name := range sum.Files {
			if strings.Contains(name, `\`) {
				t.Fatalf("momo.sum entry %q contains a backslash", name)
			}
		}
		var b strings.Builder
		if err := sum.Write(&b); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(b.String(), `\`) {
			t.Fatalf("momo.sum text contains a backslash:\n%s", b.String())
		}
	})
}
