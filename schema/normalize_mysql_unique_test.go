// SPDX-License-Identifier: Apache-2.0

package schema_test

import (
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

func mysqlTable(ix []schema.Index, un []schema.Unique) *schema.Schema {
	return &schema.Schema{Dialect: dialect.MySQL, Tables: []schema.Table{{
		Name:    "tenants",
		Columns: []schema.Column{{Name: "id", Type: "bigint"}, {Name: "name", Type: "varchar(200)"}},
		Indexes: ix, Uniques: un,
	}}}
}

// Catches: a model's unique index on plain columns differing from the UNIQUE
// constraint MySQL and MariaDB report for it, which showed as drift ("changed:
// unique ... not in snapshot", "missing: index ...") right after a clean apply.
func TestMySQLUniqueIndexEqualsUniqueConstraint(t *testing.T) {
	asIndex := mysqlTable([]schema.Index{{Name: "tenants_name_key", Unique: true, Parts: []schema.IndexPart{{Column: "name"}}}}, nil)
	asUnique := mysqlTable(nil, []schema.Unique{{Name: "tenants_name_key", Columns: []string{"name"}}})
	if !schema.Equal(asIndex, asUnique) {
		t.Error("a plain unique index and a unique constraint must compare equal on MySQL")
	}
	n := schema.Normalized(asIndex)
	if len(n.Tables[0].Indexes) != 0 || len(n.Tables[0].Uniques) != 1 {
		t.Errorf("the unique index was not folded: %+v", n.Tables[0])
	}
	if again := schema.Normalized(n); !schema.Equal(again, n) {
		t.Error("folding is not idempotent")
	}
	// The same declaration on PostgreSQL keeps its two spellings apart.
	pgIndex, pgUnique := mysqlTable(asIndex.Tables[0].Indexes, nil), mysqlTable(nil, asUnique.Tables[0].Uniques)
	pgIndex.Dialect, pgUnique.Dialect = dialect.Postgres, dialect.Postgres
	if schema.Equal(pgIndex, pgUnique) {
		t.Error("PostgreSQL must keep a unique index and a unique constraint apart")
	}
}

// Catches: the fold swallowing a unique index that is not a constraint on the
// server (an expression, a descending part or a predicate), which would hide a
// real difference.
func TestMySQLUniqueIndexKeepsSpecialForms(t *testing.T) {
	for name, ix := range map[string]schema.Index{
		"expression": {Name: "i", Unique: true, Parts: []schema.IndexPart{{Expr: "lower(name)"}}},
		"desc":       {Name: "i", Unique: true, Parts: []schema.IndexPart{{Column: "name", Desc: true}}},
		"prefix":     {Name: "i", Unique: true, Parts: []schema.IndexPart{{Column: "name(10)"}}},
		"partial":    {Name: "i", Unique: true, Parts: []schema.IndexPart{{Column: "name"}}, Where: "id > 0"},
		"plain":      {Name: "i", Parts: []schema.IndexPart{{Column: "name"}}},
	} {
		n := schema.Normalized(mysqlTable([]schema.Index{ix}, nil))
		if len(n.Tables[0].Indexes) != 1 || len(n.Tables[0].Uniques) != 0 {
			t.Errorf("%s: the index was folded into a unique constraint: %+v", name, n.Tables[0])
		}
	}
}
