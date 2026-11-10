// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

// TestIdentityAlwaysTag is issue 56: identity:always declares GENERATED
// ALWAYS AS IDENTITY, identity and identity:by_default stay by default, and the
// column is still an identity column for inserts.
func TestIdentityAlwaysTag(t *testing.T) {
	g, diags, err := Load([]string{"./identityalways"}, Config{Dir: fixturesDir(t)})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	s, diags := BuildSchema(g.Models, pg())
	if len(diags) != 0 {
		t.Fatalf("schema diagnostics: %v", diags)
	}
	if c := mustCol(t, s, "ledger", "seq"); c.Identity != "always" {
		t.Errorf("ledger.seq identity = %q, want always", c.Identity)
	}
	if c := mustCol(t, s, "counter", "id"); c.Identity != "by_default" {
		t.Errorf("counter.id identity = %q, want by_default", c.Identity)
	}
	if c := mustCol(t, s, "counter", "hits"); c.Identity != "by_default" {
		t.Errorf("counter.hits identity = %q, want by_default", c.Identity)
	}
	for _, m := range g.Models {
		if m.GoName == "Ledger" && !m.Fields[0].Tags.Identity {
			t.Error("an always column must still count as an identity column, so inserts omit it")
		}
	}
}

func TestIdentityAlwaysRefusedOnMySQL(t *testing.T) {
	g, diags, err := Load([]string{"./identityalways"}, Config{Dir: fixturesDir(t)})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	_, diags = BuildSchema(g.Models, Config{Dialects: []dialect.Name{dialect.Postgres, dialect.MySQL}})
	if !strings.Contains(diagStrings(diags), "identity:always is not supported on mysql") {
		t.Fatalf("diagnostics = %v", diags)
	}
	_, diags = BuildSchema(g.Models, Config{Dialects: []dialect.Name{dialect.Postgres, dialect.SQLite}})
	if len(diags) != 0 {
		t.Fatalf("SQLite must accept identity:always: %v", diags)
	}
}
