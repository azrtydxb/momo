// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"path/filepath"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/gen/naming"
	"github.com/azrtydxb/momo/schema"
)

// prismaSchema loads the prisma fixture with the prisma preset and the given
// type defaults, the way a Prisma project configures momo.yaml.
func prismaSchema(t *testing.T, typeDefaults map[string]string) *schema.Schema {
	t.Helper()
	dir, err := filepath.Abs("testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{Dir: dir, Naming: naming.Prisma{}, TypeDefaults: typeDefaults, Dialects: []dialect.Name{dialect.Postgres}}
	g, diags, err := Load([]string{"./prisma"}, cfg)
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	return g.Schema
}

// TestPrismaPresetSchema catches a table, column, key or index name that
// drifts from what Prisma creates, and a lost time.Time type default.
func TestPrismaPresetSchema(t *testing.T) {
	s := prismaSchema(t, map[string]string{"time.Time": "timestamp(3)"})
	chunk, ok := s.Table("KbChunk")
	if !ok {
		t.Fatalf("table KbChunk missing; tables=%v", s.Tables)
	}
	for col, typ := range map[string]string{
		"id": "text", "accountId": "text", "createdAt": "timestamp(3)", "deletedAt": "timestamp(3)",
	} {
		c, found := chunk.Column(col)
		if !found {
			t.Fatalf("column %q missing", col)
		}
		if c.Type != typ {
			t.Errorf("column %q type=%q want %q", col, c.Type, typ)
		}
	}
	if c, _ := chunk.Column("deletedAt"); !c.Nullable {
		t.Error("deletedAt must be nullable")
	}
	if _, ok := chunk.Index("KbChunk_accountId_idx"); !ok {
		t.Errorf("index KbChunk_accountId_idx missing; indexes=%v", chunk.Indexes)
	}
	if len(chunk.ForeignKeys) != 1 || chunk.ForeignKeys[0].Name != "KbChunk_accountId_fkey" || chunk.ForeignKeys[0].RefTable != "Account" {
		t.Errorf("foreign keys=%+v", chunk.ForeignKeys)
	}
	acct, ok := s.Table("Account")
	if !ok {
		t.Fatal("table Account missing")
	}
	if ix, ok := acct.Index("Account_email_key"); !ok || !ix.Unique {
		t.Errorf("unique index Account_email_key missing; indexes=%v", acct.Indexes)
	}
}

// TestTypeDefaultsOptional catches a type default that leaks when unset:
// without it time.Time stays timestamptz.
func TestTypeDefaultsOptional(t *testing.T) {
	s := prismaSchema(t, nil)
	acct, _ := s.Table("Account")
	c, _ := acct.Column("createdAt")
	if c.Type != "timestamp with time zone" {
		t.Fatalf("createdAt type=%q want timestamp with time zone", c.Type)
	}
}
