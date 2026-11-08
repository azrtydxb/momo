// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

func TestPostgresConcurrentIndexNoTx(t *testing.T) {
	stmts, err := mpg.Dialect{}.Render([]migrate.Change{{Kind: "create_index", Table: "docs", Name: "i", After: &schema.Index{Name: "i", Concurrently: true, Parts: []schema.IndexPart{{Column: "x"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	if !stmts[0].NoTx || !strings.HasPrefix(stmts[0].SQL, "CREATE INDEX CONCURRENTLY") {
		t.Fatalf("concurrent index must render outside a transaction: %+v", stmts[0])
	}
}
