// SPDX-License-Identifier: Apache-2.0

// Package exampletest is the harness every examples/<name> test uses. It
// gives the test a database that has the example's migrations applied, so the
// example is proven against a real server by the same CI job that runs the
// integration suite.
package exampletest

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
)

const applyTimeout = 2 * time.Minute

// Open creates a scratch PostgreSQL database with the vector extension,
// applies the migrations in migrationsDir (relative to the example, normally
// "migrations") and returns a handle on it. The database is dropped when the
// test ends. The test is skipped when MOMO_TEST_POSTGRES_URL is unset, and
// fails instead when MOMO_REQUIRE_INTEGRATION=1, as in CI.
func Open(t *testing.T, migrationsDir string) *momo.DB {
	t.Helper()
	db := testopen.PostgresIn(t, testdb.ScratchDatabase(t, "ex_"))
	e, err := migrate.NewEngine(db.Driver(), os.DirFS(migrationsDir), migrate.WithDialect(mpg.New()))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), applyTimeout)
	defer cancel()
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatalf("apply %s: %v", migrationsDir, err)
	}
	return db
}

// CheckGenerated fails when the generated code of the example in the working
// directory is stale, by running the real generator in check mode. It is the
// test-side twin of `momo generate`, so a model edit without a regenerate
// cannot reach main.
func CheckGenerated(t *testing.T) {
	t.Helper()
	cmd := exec.Command("go", "run", "github.com/azrtydxb/momo/cmd/momo", "generate", "--check") //nolint:gosec // fixed arguments
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("momo generate --check: %v\n%s", err, out)
	}
}
