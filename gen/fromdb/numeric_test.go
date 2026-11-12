// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

// Catches: --from-db mapping numeric to types.Numeric but dropping the
// precision and scale of numeric(p,s), which would regenerate a different
// column.
func TestGenerateFromDBNumericKeepsPrecisionAndScale(t *testing.T) {
	s := &schema.Schema{
		FormatVersion: schema.FormatVersion,
		Dialect:       dialect.Postgres,
		Tables: []schema.Table{{
			Name: "prices",
			Columns: []schema.Column{
				{Name: "id", Type: "bigint", Identity: "by_default"},
				{Name: "amount", Type: "numeric(12,2)"},
				{Name: "whole", Type: "numeric(5)"},
				{Name: "free", Type: "numeric"},
				{Name: "maybe", Type: "numeric(7,3)", Nullable: true},
			},
			PrimaryKey: []string{"id"},
		}},
	}
	res, files := generate(t, s, Config{})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	src := files["prices"]
	mustMatch(t, "prices", src,
		`Amount\s+types\.Numeric\s+`+"`"+`momo:"precision:12,scale:2"`,
		`Whole\s+types\.Numeric\s+`+"`"+`momo:"precision:5"`,
		`Free\s+types\.Numeric`,
		`Maybe\s+\*?types\.Numeric\s+`+"`"+`momo:"[^"]*precision:7,scale:3`,
	)
	for _, line := range strings.Split(src, "\n") {
		if strings.Contains(line, "Free") && strings.Contains(line, "precision") {
			t.Errorf("an unconstrained numeric must not gain a precision tag: %s", line)
		}
	}
}
