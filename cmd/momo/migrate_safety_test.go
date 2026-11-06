// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

const unreachableURL = "postgres://alice:hunter2secret@127.0.0.1:1/none?sslmode=disable&connect_timeout=1"

func safetyConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return yamlPath(t, dir, "migrate:\n  dir:\n    postgres: "+filepath.ToSlash(dir)+"/m\n")
}

// --force-unlock did nothing on every dialect; it is gone, and the help no
// longer offers it.
func TestMigrateForceUnlockIsRemoved(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runMigrate(&out, &errOut, []string{"--config", safetyConfig(t), "apply", "--force-unlock", "--url", unreachableURL}, noEnv)
	if code != 2 || !strings.Contains(errOut.String(), "force-unlock") {
		t.Fatalf("exit %d, want 2 (unknown flag): %s", code, errOut.String())
	}
	if strings.Contains(migrateUsage, "--force-unlock") {
		t.Fatal("the usage text still documents --force-unlock")
	}
	if !strings.Contains(migrateUsage, "pg_terminate_backend") {
		t.Fatal("the usage text must say how to free a stuck PostgreSQL lock")
	}
}

func TestMigrateURLRoutes(t *testing.T) {
	cfg := safetyConfig(t)
	t.Run("--url warns about the process list", func(t *testing.T) {
		var out, errOut bytes.Buffer
		runMigrate(&out, &errOut, []string{"--config", cfg, "status", "--url", unreachableURL}, noEnv)
		if !strings.Contains(errOut.String(), "process list") || !strings.Contains(errOut.String(), "MOMO_DATABASE_URL") {
			t.Fatalf("want a warning naming the safer routes, got %q", errOut.String())
		}
		if strings.Contains(out.String()+errOut.String(), "hunter2secret") {
			t.Fatalf("output leaks the password: %q", out.String()+errOut.String())
		}
	})
	t.Run("the environment variable needs no warning", func(t *testing.T) {
		var out, errOut bytes.Buffer
		env := func(k string) string {
			if k == "MOMO_DATABASE_URL" {
				return unreachableURL
			}
			return ""
		}
		code := runMigrate(&out, &errOut, []string{"--config", cfg, "status"}, env)
		if code != 3 || strings.Contains(errOut.String(), "process list") {
			t.Fatalf("exit %d (want 3, unreachable), stderr %q", code, errOut.String())
		}
	})
	t.Run("--url - reads standard input", func(t *testing.T) {
		old := migrateStdin
		migrateStdin = strings.NewReader(unreachableURL + "\n")
		t.Cleanup(func() { migrateStdin = old })
		var out, errOut bytes.Buffer
		code := runMigrate(&out, &errOut, []string{"--config", cfg, "status", "--url", "-"}, noEnv)
		if code != 3 {
			t.Fatalf("exit %d, want 3 (the URL from stdin was used and is unreachable): %s", code, errOut.String())
		}
		if all := out.String() + errOut.String(); strings.Contains(all, "process list") || strings.Contains(all, "hunter2secret") {
			t.Fatalf("stdin route warned or leaked: %q", all)
		}
	})
	t.Run("--url - with empty input is a usage error", func(t *testing.T) {
		old := migrateStdin
		migrateStdin = strings.NewReader("  \n")
		t.Cleanup(func() { migrateStdin = old })
		var out, errOut bytes.Buffer
		if code := runMigrate(&out, &errOut, []string{"--config", cfg, "status", "--url", "-"}, noEnv); code != 2 {
			t.Fatalf("exit %d, want 2: %s", code, errOut.String())
		}
	})
}

func TestMigrateRehashUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runMigrate(&out, &errOut, []string{"--config", safetyConfig(t), "rehash", "--url", unreachableURL}, noEnv); code != 2 {
		t.Fatalf("rehash without --version: exit %d, want 2: %s", code, errOut.String())
	}
	if !strings.Contains(migrateUsage, "rehash --version N [--accept]") {
		t.Fatal("rehash is missing from the usage text")
	}
}

// recDialect records the session options the engine passes to Setup. It
// reports the SQLite dialect so the engine does not query current_schema().
type recDialect struct{ got *migrate.SessionOptions }

func (recDialect) Dialect() *dialect.Dialect { return dialect.For(dialect.SQLite) }
func (recDialect) Inspect(context.Context, momo.PinnedConn, migrate.InspectOptions) (*schema.Schema, error) {
	return &schema.Schema{}, nil
}
func (recDialect) Render([]migrate.Change) ([]migrate.Statement, error) { return nil, nil }
func (recDialect) HistoryDDL() []string                                 { return nil }
func (d recDialect) Setup(_ context.Context, _ momo.PinnedConn, s migrate.SessionOptions) (func() error, error) {
	*d.got = s
	return func() error { return nil }, nil
}
func (recDialect) Lock(context.Context, momo.PinnedConn, string) (func() error, func(context.Context) (string, error), error) {
	return func() error { return nil }, nil, nil
}

type noTable struct{}

func (noTable) SQLState() string { return "42P01" }
func (noTable) Error() string    { return "no table" }

type recDriver struct{}

func (recDriver) Dialect() *dialect.Dialect                               { return dialect.For(dialect.SQLite) }
func (recDriver) Query(context.Context, string, []any) (momo.Rows, error) { return nil, noTable{} }
func (recDriver) Exec(context.Context, string, []any) (int64, error)      { return 0, nil }
func (recDriver) Begin(context.Context, momo.TxOptions) (momo.DriverTx, error) {
	return nil, errors.New("no tx")
}
func (d recDriver) Pin(context.Context) (momo.PinnedConn, error) { return recPinned{d}, nil }

type recPinned struct{ recDriver }

func (recPinned) Release() error { return nil }

// A timeout key in momo.yaml, including 0s, reaches the migration session.
func TestMigrateTimeoutKeysReachTheSession(t *testing.T) {
	for name, tc := range map[string]struct {
		yaml string
		want migrate.SessionOptions
	}{
		"absent": {"", migrate.SessionOptions{LockTimeout: 5 * time.Second}},
		"values": {"  lock_timeout: 2s\n  statement_timeout: 30s\n", migrate.SessionOptions{LockTimeout: 2 * time.Second, StatementTimeout: 30 * time.Second, LockTimeoutSet: true, StatementTimeoutSet: true}},
		"zero":   {"  lock_timeout: 0s\n  statement_timeout: 0s\n", migrate.SessionOptions{LockTimeoutSet: true, StatementTimeoutSet: true}},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			cfg := yamlPath(t, dir, "migrate:\n"+tc.yaml)
			l := &cli{}
			file, _, err := loadConfigFile(cfg, dir)
			if err != nil {
				t.Fatal(err)
			}
			l.file = file
			var got migrate.SessionOptions
			d := recDialect{&got}
			opts, err := l.engineOptions(d)
			if err != nil {
				t.Fatal(err)
			}
			e, err := migrate.NewEngine(recDriver{}, fstest.MapFS{}, append(opts, migrate.WithoutSum(), migrate.WithLockWait(0))...)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.Apply(context.Background(), migrate.ApplyOptions{}); err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("session options = %+v, want %+v", got, tc.want)
			}
		})
	}
}
