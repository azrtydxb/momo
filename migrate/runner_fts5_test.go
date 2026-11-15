// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
)

type failingExecer struct{ err error }

func (f failingExecer) Exec(context.Context, string, []any) (int64, error) { return 0, f.err }

// Catches: a SQLite build without FTS5 surfacing the raw driver string "no such
// module: fts5" instead of ErrUnsupported naming the missing module, and the
// mapping firing for a statement that is not the FTS5 table.
func TestExecStatementFTS5Missing(t *testing.T) {
	raw := errors.New("no such module: fts5")
	err := execStatement(context.Background(), failingExecer{raw}, Statement{
		SQL: `CREATE VIRTUAL TABLE "articles_fts" USING fts5("title")`, Hint: "requires:fts5"})
	if !errors.Is(err, momo.ErrUnsupported) || !strings.Contains(err.Error(), "FTS5") || strings.Contains(err.Error(), "no such module") {
		t.Fatalf("err = %v, want ErrUnsupported naming FTS5 without the driver text", err)
	}
	// A migration file carries no hint; the statement text identifies the table.
	err = execStatement(context.Background(), failingExecer{raw}, Statement{SQL: `CREATE VIRTUAL TABLE "a_fts" USING fts5("t")`})
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("unhinted err = %v, want ErrUnsupported", err)
	}
	err = execStatement(context.Background(), failingExecer{raw}, Statement{SQL: "SELECT 1"})
	if errors.Is(err, momo.ErrUnsupported) || !errors.Is(err, raw) {
		t.Fatalf("unrelated statement err = %v, want the raw error", err)
	}
}
