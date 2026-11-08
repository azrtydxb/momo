// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/gen/naming"
)

// TestValidTimeType catches an allowlist that lets a value with SQL syntax
// through, or rejects a legitimate time type.
func TestValidTimeType(t *testing.T) {
	for _, ok := range []string{"timestamp", "timestamp(3)", " TIMESTAMPTZ(6) ", "timestamp(0) without time zone", "timestamp with time zone", "time(2)", "timetz"} {
		if !ValidTimeType(ok) {
			t.Errorf("ValidTimeType(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"", "text", "timestamp(3); DROP TABLE x", "timestamp'", `"timestamp"`, "timestamp(7)", "timestamp(3) --", "date", "timestamp(3)\ntext"} {
		if ValidTimeType(bad) {
			t.Errorf("ValidTimeType(%q) = true, want false", bad)
		}
	}
}

// TestTypeDefaultInvalidDiagnostic catches a Go-API caller whose invalid
// default reaches the schema: it must be a diagnostic, not DDL text.
func TestTypeDefaultInvalidDiagnostic(t *testing.T) {
	dir, err := filepath.Abs("testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{Dir: dir, Naming: naming.Prisma{}, TypeDefaults: map[string]string{"time.Time": "timestamp(3); DROP TABLE x"}, Dialects: []dialect.Name{dialect.Postgres}}
	g, diags, err := Load([]string{"./prisma"}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) == 0 || !strings.Contains(diags[0].Msg, "not a timestamp type") {
		t.Fatalf("want a not-a-timestamp-type diagnostic, got %v", diags)
	}
	if g != nil && g.Schema != nil {
		for _, tb := range g.Schema.Tables {
			for _, c := range tb.Columns {
				if strings.Contains(c.Type, "DROP") {
					t.Fatalf("invalid default leaked into column %s.%s: %q", tb.Name, c.Name, c.Type)
				}
			}
		}
	}
}

// TestTypeDefaultSQLitePrimary catches a Postgres type leaking into a SQLite
// schema: the default applies only when the first dialect is PostgreSQL.
func TestTypeDefaultSQLitePrimary(t *testing.T) {
	dir, err := filepath.Abs("testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{Dir: dir, Naming: naming.Prisma{}, TypeDefaults: map[string]string{"time.Time": "timestamp(3)"}, Dialects: []dialect.Name{dialect.SQLite}}
	g, _, err := Load([]string{"./prisma"}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	acct, ok := g.Schema.Table("Account")
	if !ok {
		t.Fatal("table Account missing")
	}
	if c, _ := acct.Column("createdAt"); c.Type != "TEXT" {
		t.Fatalf("createdAt type=%q want TEXT on sqlite", c.Type)
	}
}
