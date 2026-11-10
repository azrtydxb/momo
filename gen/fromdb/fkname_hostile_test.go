// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/schema"
)

// Catches: a constraint name holding a comma, colon, quote or space being
// written into a struct tag, which would corrupt the tag or the model.
func TestGenerateFromDBForeignKeyNameNotWritable(t *testing.T) {
	for _, name := range []string{"has space", "a,b", "a:b", `a"b`, "a`b", "1abc", ""} {
		s := sampleSchema()
		s.Tables[0].ForeignKeys[0].Name = name
		res, files := generate(t, s, Config{})
		if strings.Contains(files["orders"], "fk_name") {
			t.Errorf("%q: written into the model", name)
		}
		var warned bool
		for _, f := range res.Findings {
			warned = warned || f.Severity == SevWarn
		}
		if !warned {
			t.Errorf("%q: the dropped foreign key must be reported", name)
		}
	}
}

// Catches: the UPDATE OF column list being lost between the inspected schema
// and the generated models.
func TestGenerateFromDBTriggerUpdateOf(t *testing.T) {
	s := sampleSchema()
	s.Functions = []schema.Function{{Name: "touch", Returns: "trigger", Language: "plpgsql", Volatility: "VOLATILE", Body: "BEGIN RETURN NEW; END;"}}
	s.Triggers = []schema.Trigger{{
		Table: "orders", Name: "orders_total_upd", Timing: "AFTER", Events: []string{"UPDATE"},
		UpdateOf: []string{"total", "note"}, ForEachRow: true, Function: "touch()",
	}}
	res, files := generate(t, s, Config{})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	var all strings.Builder
	for _, src := range files {
		all.WriteString(src)
	}
	mustContain(t, "models", all.String(), `UpdateOf:`, `"total"`, `"note"`)
}
