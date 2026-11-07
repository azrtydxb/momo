// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

func serialSchema(d dialect.Name) *schema.Schema {
	return &schema.Schema{Dialect: d, Tables: []schema.Table{{
		Name:       "t",
		Columns:    []schema.Column{{Name: "id", Type: "bigint", Identity: "serial"}},
		PrimaryKey: []string{"id"},
	}}}
}

// Catches: a serial column in a snapshot for another dialect being rendered as
// a plain integer instead of refused.
func TestDiffRefusesSerialOutsidePostgres(t *testing.T) {
	for _, d := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		_, _, err := Diff(&schema.Schema{Dialect: d}, serialSchema(d), DiffOptions{})
		if err == nil || !strings.Contains(err.Error(), "serial, which is supported on postgres only") {
			t.Fatalf("%s: want a serial refusal, got %v", d, err)
		}
	}
	if _, _, err := Diff(&schema.Schema{Dialect: "postgres"}, serialSchema("postgres"), DiffOptions{}); err != nil {
		t.Fatalf("postgres must accept serial: %v", err)
	}
}
