// SPDX-License-Identifier: Apache-2.0

package momotest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// UpdateGoldenEnv, set to 1, makes QueryGolden write the golden files instead
// of comparing against them.
const UpdateGoldenEnv = "MOMO_UPDATE_GOLDEN"

const goldenMarker = "-- name: "

// QueryGolden freezes the SQL of a query registry in golden files, one per
// dialect (dir/postgres.sql, dir/mysql.sql, dir/sqlite.sql; PostgreSQL alone
// when no dialect is given). It needs no database. Each file lists the
// queries sorted by name:
//
//	-- name: open_tasks
//	SELECT id, title FROM tasks WHERE owner = $1 AND NOT done;
//
// The test fails when a registered query's SQL changed, a query was added, or
// one was removed, and prints the old and the new text. Run the tests with
// MOMO_UPDATE_GOLDEN=1 (UpdateGoldenEnv) to write the files after an intended
// change, then review the diff. Keep a release's files (for example
// testdata/queries/v1.4/) to run its queries against a newer schema, see
// LoadQueryGolden. Compiling a query that fails on one of the dialects fails
// the test.
func QueryGolden(t testing.TB, reg *momo.QueryRegistry, dir string, dialects ...dialect.Name) {
	t.Helper()
	if len(dialects) == 0 {
		dialects = []dialect.Name{dialect.Postgres}
	}
	update := os.Getenv(UpdateGoldenEnv) == "1"
	for _, d := range dialects {
		qs, err := reg.Compile(d)
		if err != nil {
			t.Errorf("momotest: %v", err)
			continue
		}
		path := filepath.Join(dir, string(d)+".sql")
		got := FormatQueryGolden(qs)
		if update {
			if err := os.MkdirAll(dir, 0o750); err != nil {
				t.Fatalf("momotest: %v", err)
			}
			if err := os.WriteFile(path, []byte(got), 0o600); err != nil { // #nosec G306 -- a test fixture
				t.Fatalf("momotest: %v", err)
			}
			continue
		}
		want, err := LoadQueryGolden(path)
		if err != nil {
			t.Errorf("momotest: %v (run with %s=1 to create it)", err, UpdateGoldenEnv)
			continue
		}
		if diff := diffQueries(want, qs); diff != "" {
			t.Errorf("momotest: the %s SQL of the registered queries changed (run with %s=1 to accept):\n%s", d, UpdateGoldenEnv, diff)
		}
	}
}

// FormatQueryGolden renders queries in the golden file format.
func FormatQueryGolden(qs []momo.RegisteredQuery) string {
	var b strings.Builder
	for _, q := range qs {
		b.WriteString(goldenMarker + q.Name + "\n" + q.SQL + ";\n\n")
	}
	return b.String()
}

// LoadQueryGolden reads a golden file written by QueryGolden, for example the
// file of the previous release, so a test can run those statements against
// the current schema.
func LoadQueryGolden(path string) ([]momo.RegisteredQuery, error) {
	b, err := os.ReadFile(path) // #nosec G304 -- the caller names its own fixture
	if err != nil {
		return nil, err
	}
	return ParseQueryGolden(string(b))
}

// ParseQueryGolden parses the golden file format.
func ParseQueryGolden(text string) ([]momo.RegisteredQuery, error) {
	var out []momo.RegisteredQuery
	for _, block := range strings.Split("\n"+text, "\n"+goldenMarker)[1:] {
		name, body, _ := strings.Cut(block, "\n")
		body = strings.TrimSpace(body)
		body = strings.TrimSuffix(body, ";")
		if name == "" {
			return nil, fmt.Errorf("a golden entry has no name")
		}
		out = append(out, momo.RegisteredQuery{Name: name, SQL: body})
	}
	return out, nil
}

// diffQueries describes how got differs from want, or returns "".
func diffQueries(want, got []momo.RegisteredQuery) string {
	w := make(map[string]string, len(want))
	for _, q := range want {
		w[q.Name] = q.SQL
	}
	var b strings.Builder
	seen := make(map[string]bool, len(got))
	for _, q := range got {
		seen[q.Name] = true
		old, ok := w[q.Name]
		switch {
		case !ok:
			fmt.Fprintf(&b, "+ added %s:\n    %s\n", q.Name, q.SQL)
		case old != q.SQL:
			fmt.Fprintf(&b, "~ changed %s:\n  was %s\n  now %s\n", q.Name, old, q.SQL)
		}
	}
	for _, q := range want {
		if !seen[q.Name] {
			fmt.Fprintf(&b, "- removed %s:\n    %s\n", q.Name, q.SQL)
		}
	}
	return b.String()
}
