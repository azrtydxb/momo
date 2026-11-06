// SPDX-License-Identifier: Apache-2.0

//go:build integration

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/internal/testdb"
)

const modulePath = "github.com/azrtydxb/momo"

// momoBinary builds the real CLI once per test and returns its path.
func momoBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "momo")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", bin, modulePath+"/cmd/momo").CombinedOutput(); err != nil { //nolint:gosec // fixed arguments
		t.Fatalf("go build momo: %v\n%s", err, out)
	}
	return bin
}

// cli runs the momo binary in this directory, where momo.yaml lives, against
// the database url. It returns the combined output and the exit code.
func cli(t *testing.T, bin, url string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...) //nolint:gosec // bin was built by this test
	cmd.Env = append(os.Environ(), "DATABASE_URL="+url)
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return string(out), 0
	case errors.As(err, &exit):
		return string(out), exit.ExitCode()
	default:
		t.Fatalf("momo %v: %v", args, err)
		return "", -1
	}
}

// mustCLI runs momo and fails the test unless it exits 0.
func mustCLI(t *testing.T, bin, url string, args ...string) string {
	t.Helper()
	out, code := cli(t, bin, url, args...)
	if code != 0 {
		t.Fatalf("momo %v exited %d:\n%s", args, code, out)
	}
	return out
}

// versions returns the migration versions in migrations/, oldest first.
func versions(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("migrations/*.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, f := range files {
		out = append(out, strings.SplitN(filepath.Base(f), "_", 2)[0])
	}
	sort.Strings(out)
	return out
}

// legacyDatabase creates a database with nothing but legacy.sql in it: no
// Momo history table, no migrations.
func legacyDatabase(ctx context.Context, t *testing.T) (url string, conn *pgx.Conn) {
	t.Helper()
	url = testdb.PostgresURLFor(t, testdb.ScratchDatabase(t, "adopt_"))
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(ctx) })
	// The test helper installs pgvector in every scratch database. This schema
	// does not use it, and drift would report it as unmanaged.
	if _, err := conn.Exec(ctx, `DROP EXTENSION vector`); err != nil {
		t.Fatal(err)
	}
	legacy, err := os.ReadFile("legacy.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, string(legacy)); err != nil {
		t.Fatal(err)
	}
	return url, conn
}

func TestAdoptLegacyDatabase(t *testing.T) {
	ctx := context.Background()
	url, conn := legacyDatabase(ctx, t)
	bin := momoBinary(t)
	v := versions(t)
	if len(v) != 2 {
		t.Fatalf("want 2 migrations (init, add_order_status), found %v", v)
	}
	mustCLI(t, bin, url, "migrate", "validate")

	// Applying init would fail: the tables exist. Baseline records it as
	// applied instead, after checking that the live database matches the
	// snapshot of that version. A database that differs is refused, exit 1.
	if _, err := conn.Exec(ctx, `ALTER TABLE users ADD COLUMN nickname text`); err != nil {
		t.Fatal(err)
	}
	out, code := cli(t, bin, url, "migrate", "baseline", "--version", v[0])
	if code != 1 || !strings.Contains(out, "does not match the snapshot") {
		t.Fatalf("baseline of a differing database: exit=%d, want 1 and a mismatch:\n%s", code, out)
	}
	if _, err := conn.Exec(ctx, `ALTER TABLE users DROP COLUMN nickname`); err != nil {
		t.Fatal(err)
	}
	mustCLI(t, bin, url, "migrate", "baseline", "--version", v[0])

	// From here the database is a normal Momo database. Only the migration
	// made after the baseline is pending.
	out = mustCLI(t, bin, url, "migrate", "plan")
	if strings.Count(out, "pending ") != 1 || !strings.Contains(out, "add_order_status") {
		t.Fatalf("want only add_order_status pending:\n%s", out)
	}
	mustCLI(t, bin, url, "migrate", "apply")
	mustCLI(t, bin, url, "migrate", "status")
	if out := mustCLI(t, bin, url, "migrate", "drift"); !strings.Contains(out, "no drift") {
		t.Fatalf("unexpected drift:\n%s", out)
	}

	// The legacy rows are intact and the new column holds its default.
	pool, err := pgxdriver.NewPool(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := run(ctx, momo.New(pgxdriver.New(pool))); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM orders WHERE status = 'new'`).Scan(&n); err != nil || n != 3 {
		t.Fatalf("orders with status new: %d, %v; want 3", n, err)
	}
}

// TestNewDatabaseGetsTheSameSchema shows the other side of adoption: a
// database that never had legacy.sql is built from the same migrations and
// ends with the same schema, so every environment converges.
func TestNewDatabaseGetsTheSameSchema(t *testing.T) {
	ctx := context.Background()
	url := testdb.PostgresURLFor(t, testdb.ScratchDatabase(t, "adopt_new_"))
	bin := momoBinary(t)
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	if _, err := conn.Exec(ctx, `DROP EXTENSION vector`); err != nil {
		t.Fatal(err)
	}
	mustCLI(t, bin, url, "migrate", "apply")
	if out := mustCLI(t, bin, url, "migrate", "drift"); !strings.Contains(out, "no drift") {
		t.Fatalf("unexpected drift:\n%s", out)
	}
}
