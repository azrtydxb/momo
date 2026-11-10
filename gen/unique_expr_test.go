// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/schema"
)

func exprIndex(t *testing.T, tb *schema.Table, name string) *schema.Index {
	t.Helper()
	ix, ok := tb.Index(name)
	if !ok {
		t.Fatalf("index %s missing from table %s: %+v", name, tb.Name, tb.Indexes)
	}
	return ix
}

// TestCompanionUniqueExpressionIndex is issue 55: IndexExpr(...).Unique()
// declares a unique expression index, an argument naming a column is a plain
// column key, and Where makes it partial.
func TestCompanionUniqueExpressionIndex(t *testing.T) {
	g, diags, err := Load([]string{"./companionuniqexpr"}, Config{Dir: fixturesDir(t)})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	s, diags := BuildSchema(g.Models, pg())
	if len(diags) != 0 {
		t.Fatalf("schema diagnostics: %v", diags)
	}
	pr, _ := s.Table("principal")
	ix := exprIndex(t, pr, "principal_email_key")
	if !ix.Unique || len(ix.Parts) != 1 || ix.Parts[0].Expr != "lower(email)" || ix.Parts[0].Column != "" {
		t.Fatalf("principal_email_key = %+v", ix)
	}
	if len(pr.Uniques) != 0 {
		t.Fatalf("a unique expression index must not be a constraint: %+v", pr.Uniques)
	}
	tp, _ := s.Table("template")
	mixed := exprIndex(t, tp, "template_company_name_key")
	if !mixed.Unique || len(mixed.Parts) != 2 || mixed.Parts[0].Column != "company_id" || mixed.Parts[0].Expr != "" ||
		mixed.Parts[1].Expr != "lower(name)" {
		t.Fatalf("template_company_name_key = %+v", mixed)
	}
	live := exprIndex(t, tp, "template_live_name_key")
	if !live.Unique || live.Where != "deleted_at IS NULL" {
		t.Fatalf("template_live_name_key = %+v", live)
	}
	if plain := exprIndex(t, tp, "template_plain_expr_idx"); plain.Unique {
		t.Fatalf("an IndexExpr without Unique must stay non-unique: %+v", plain)
	}
}

func TestCompanionUniqueTwiceIsRefused(t *testing.T) {
	_, diags, err := Load([]string{"./companionuniqbad"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diagStrings(diags), `index "twice_email_key" is already unique`) {
		t.Fatalf("diagnostics = %v", diags)
	}
}
