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
	"github.com/azrtydxb/momo/examples/upgrade-database/models"
	"github.com/azrtydxb/momo/examples/upgrade-database/models/customer"
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

func TestUpgradeKeepsData(t *testing.T) {
	ctx := context.Background()
	url := testdb.PostgresURLFor(t, scratch(t))
	bin := momoBinary(t)
	v := versions(t)
	if len(v) != 3 {
		t.Fatalf("want 3 migrations (init, add_plan, rename_name), found %v", v)
	}

	// Offline checks: the committed files, and the models against them.
	mustCLI(t, bin, url, "migrate", "validate")
	if out := mustCLI(t, bin, url, "migrate", "diff", "check"); !strings.Contains(out, "no changes") {
		t.Fatalf("the models and the migrations differ:\n%s", out)
	}
	mustCLI(t, bin, url, "generate", "--check")

	// Schema v1 is the database an existing deployment runs.
	mustCLI(t, bin, url, "migrate", "apply", "--to", v[0])
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	// The test helper installs pgvector in every scratch database. This schema
	// does not use it, and drift would report it as unmanaged.
	if _, err := conn.Exec(ctx, `DROP EXTENSION vector`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO customers (email, name) VALUES ('ada@example.com', 'Ada')`); err != nil {
		t.Fatal(err)
	}

	// The upgrade: plan shows what is pending, apply runs it.
	if out := mustCLI(t, bin, url, "migrate", "plan"); strings.Count(out, "pending ") != 2 {
		t.Fatalf("want 2 pending migrations:\n%s", out)
	}
	mustCLI(t, bin, url, "migrate", "apply")

	// The row survived and the new column took its default.
	pool, err := pgxdriver.NewPool(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	db := momo.New(pgxdriver.New(pool))
	ada, err := momo.Select[models.Customer](db).Where(customer.Email.Eq("ada@example.com")).One(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if ada.Name != "Ada" || ada.Plan != "free" {
		t.Fatalf("after the upgrade got name=%q plan=%q, want Ada and free", ada.Name, ada.Plan)
	}
	var indexes int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM pg_indexes WHERE indexname = 'customers_plan_idx'`).Scan(&indexes); err != nil || indexes != 1 {
		t.Fatalf("index customers_plan_idx: count=%d err=%v", indexes, err)
	}
	if err := run(ctx, db); err != nil {
		t.Fatal(err)
	}

	// Applied migrations and the live database agree.
	mustCLI(t, bin, url, "migrate", "status")
	if out := mustCLI(t, bin, url, "migrate", "drift"); !strings.Contains(out, "no drift") {
		t.Fatalf("unexpected drift:\n%s", out)
	}
	if out := mustCLI(t, bin, url, "migrate", "apply"); !strings.Contains(out, "no pending migrations") {
		t.Fatalf("a second apply must do nothing:\n%s", out)
	}

	// A change made outside Momo is drift: exit code 1.
	if _, err := conn.Exec(ctx, `ALTER TABLE customers ADD COLUMN legacy integer`); err != nil {
		t.Fatal(err)
	}
	out, code := cli(t, bin, url, "migrate", "drift")
	if code != 1 || !strings.Contains(out, "legacy") {
		t.Fatalf("drift exit=%d, want 1 naming the column:\n%s", code, out)
	}
}

// scratch creates an empty database for this test and returns its name.
func scratch(t *testing.T) string { return testdb.ScratchDatabase(t, "upgrade_") }
