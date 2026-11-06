// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/examples/internal/exampletest"
	"github.com/azrtydxb/momo/internal/testdb"
)

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

func TestCustomMigrationsRunBesideGeneratedOnes(t *testing.T) {
	ctx := context.Background()
	url := testdb.PostgresURLFor(t, testdb.ScratchDatabase(t, "custom_sql_"))
	bin := exampletest.MomoBinary(t)

	// The directory is valid: every file is in momo.sum, every migration has a
	// snapshot, and the custom ones leave the schema as their parent had it.
	mustCLI(t, bin, url, "migrate", "validate")

	// Apply up to the generated migration that adds the column, then insert
	// rows the way an existing deployment would hold them.
	mustCLI(t, bin, url, "migrate", "apply", "--to", "20261106132433")
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
	if _, err := conn.Exec(ctx, `INSERT INTO accounts (email) VALUES ('Ada@Example.com'), ('bob@example.com')`); err != nil {
		t.Fatal(err)
	}

	// plan counts statements after splitting: the backfill file holds an
	// UPDATE and one DO block, whose semicolons stay inside its $$ body.
	plan := mustCLI(t, bin, url, "migrate", "plan")
	if !strings.Contains(plan, "backfill_email_norm (2 statements)") {
		t.Fatalf("the backfill file should split into 2 statements:\n%s", plan)
	}
	mustCLI(t, bin, url, "migrate", "apply")

	// The backfill ran before the unique index was built.
	var norm string
	if err := conn.QueryRow(ctx, `SELECT email_norm FROM accounts WHERE email = 'Ada@Example.com'`).Scan(&norm); err != nil || norm != "ada@example.com" {
		t.Fatalf("email_norm = %q, %v; want ada@example.com", norm, err)
	}
	var valid bool
	if err := conn.QueryRow(ctx, `SELECT indisvalid FROM pg_index WHERE indexrelid = 'accounts_email_norm_key'::regclass`).Scan(&valid); err != nil || !valid {
		t.Fatalf("index accounts_email_norm_key valid=%v err=%v", valid, err)
	}

	// Every migration is recorded as applied, and the live schema matches the
	// last snapshot: the custom migrations did not change the schema.
	if out := mustCLI(t, bin, url, "migrate", "status"); strings.Count(out, "applied ") != 5 {
		t.Fatalf("want 5 applied migrations:\n%s", out)
	}
	if out := mustCLI(t, bin, url, "migrate", "drift"); !strings.Contains(out, "no drift") {
		t.Fatalf("unexpected drift:\n%s", out)
	}

	pool, err := pgxdriver.NewPool(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := run(ctx, momo.New(pgxdriver.New(pool))); err != nil {
		t.Fatal(err)
	}
}

// TestEditedMigrationIsRefused shows the checksum guard: changing an applied
// or pending file without re-recording it in momo.sum fails validation.
func TestEditedMigrationIsRefused(t *testing.T) {
	bin := exampletest.MomoBinary(t)
	dir := t.TempDir()
	files, err := filepath.Glob("migrations/*")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		body, err := os.ReadFile(f) //nolint:gosec // files of this example
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(f, "backfill_email_norm.up.sql") {
			body = append(body, "-- an edit nobody recorded\n"...)
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.Base(f)), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	out, code := cli(t, bin, "", "migrate", "validate", "--dir", dir)
	if code != 1 || !strings.Contains(out, "momo.sum mismatch") {
		t.Fatalf("validate exit=%d, want 1 naming a momo.sum mismatch:\n%s", code, out)
	}
}
