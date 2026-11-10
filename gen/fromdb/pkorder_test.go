// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

// Catches: a key whose column order differs from the table's column order being
// left to the baseline to report instead of declared with c.PrimaryKey.
func TestGenerateFromDBPrimaryKeyOrder(t *testing.T) {
	s := &schema.Schema{
		FormatVersion: schema.FormatVersion,
		Dialect:       dialect.Postgres,
		Tables: []schema.Table{{
			Name: "health_sample",
			Columns: []schema.Column{
				{Name: "at", Type: "timestamp with time zone"},
				{Name: "scope", Type: "text"},
				{Name: "name", Type: "text"},
				{Name: "value", Type: "double precision"},
			},
			PrimaryKey: []string{"name", "scope", "at"},
		}},
	}
	res, files := generate(t, s, Config{})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	for _, f := range res.Findings {
		if f.Severity == SevWarn {
			t.Errorf("the key is declarable, no warning expected: %v", f)
		}
	}
	mustContain(t, "health_sample model", files["health_sample"], "c.PrimaryKey(&m.Name, &m.Scope, &m.At)")
}
