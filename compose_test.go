// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

func TestScopeAppliesInOrder(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.Postgres))
	byTenant := func(id int64) momo.Scope[unitDocument] {
		return func(q momo.SelectQuery[unitDocument]) momo.SelectQuery[unitDocument] {
			return q.Where(DocTenantID.Eq(id))
		}
	}
	base := momo.Select[unitDocument](db)
	c, err := base.Scope(byTenant(2), nil, func(q momo.SelectQuery[unitDocument]) momo.SelectQuery[unitDocument] {
		return q.OrderBy(DocTitle.Asc())
	}).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.SQL, `"documents"."tenant_id" = $1`) || !strings.Contains(c.SQL, `ORDER BY "documents"."title" ASC`) || len(c.Args) != 1 || c.Args[0] != int64(2) {
		t.Fatalf("SQL = %s args = %v", c.SQL, c.Args)
	}
	// The scope must not mutate the base query.
	if plain, _ := base.SQL(); strings.Contains(plain.SQL, "tenant_id") {
		t.Fatalf("base query changed: %s", plain.SQL)
	}
}

// A filter tree renders only allowlisted columns, quoted by the builder, and
// binds every value. It fails if a field name or value reaches the SQL text.
func TestFilterPredicateRendering(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.Postgres))
	al := momo.NewAllowlist(DocTenantID, DocTitle)
	f := momo.Filter{And: []momo.Filter{
		{Or: []momo.Filter{
			{Field: "tenant_id", Op: "eq", Value: int64(1)},
			{Field: "title", Op: "in", Value: []string{"c", "d"}},
		}},
		{Not: &momo.Filter{Field: "title", Op: "like", Value: "x!%"}},
		{Field: "tenant_id", Op: "ge", Value: int64(0)},
		{Field: "title", Op: "notnull"},
	}}
	p, err := f.Predicate(al)
	if err != nil {
		t.Fatal(err)
	}
	c, err := momo.Select[unitDocument](db).Where(p).SQL()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`("documents"."tenant_id" = $1 OR "documents"."title" IN ($2, $3))`,
		`NOT ("documents"."title" LIKE $4 ESCAPE '!')`,
		`"documents"."tenant_id" >= $5`,
		`"documents"."title" IS NOT NULL`,
	} {
		if !strings.Contains(c.SQL, want) {
			t.Fatalf("SQL lacks %s:\n%s", want, c.SQL)
		}
	}
	if len(c.Args) != 5 || c.Args[3] != "x!%" {
		t.Fatalf("args = %v", c.Args)
	}
}

func TestFilterRejectsBadInput(t *testing.T) {
	al := momo.NewAllowlist(DocTenantID, DocTitle)
	bad := map[string]momo.Filter{
		"unknown field":  {Field: `tenant_id"; drop table documents; --`, Op: "eq", Value: 1},
		"unknown op":     {Field: "tenant_id", Op: "eq; drop", Value: 1},
		"empty node":     {},
		"two forms":      {Field: "tenant_id", Op: "eq", Value: 1, Not: &momo.Filter{Field: "title", Op: "null"}},
		"nil comparison": {Field: "tenant_id", Op: "eq"},
		"like non-text":  {Field: "title", Op: "like", Value: 5},
		"in non-slice":   {Field: "title", Op: "in", Value: "a"},
		"nested unknown": {Or: []momo.Filter{{Field: "title", Op: "null"}, {Field: "nope", Op: "null"}}},
	}
	for name, f := range bad {
		_, err := f.Predicate(al)
		if !errors.Is(err, momo.ErrValidation) {
			t.Fatalf("%s: err = %v, want ErrValidation", name, err)
		}
		if strings.Contains(err.Error(), "drop") {
			t.Fatalf("%s: error echoes input: %v", name, err)
		}
	}
	deep := momo.Filter{Field: "title", Op: "null"}
	for i := 0; i < 100; i++ {
		d := deep
		deep = momo.Filter{Not: &d}
	}
	if _, err := deep.Predicate(al); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("deep tree err = %v, want ErrValidation", err)
	}
}
