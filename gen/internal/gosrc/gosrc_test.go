// SPDX-License-Identifier: Apache-2.0

package gosrc

import "testing"

// Catches: a model package named like a local of the generated code (data,
// keys, rows, out, in, q, sel, res), which the generated function would shadow
// and fail to compile.
func TestAliasesAvoidTemplateLocals(t *testing.T) {
	a := NewAliases(map[string]string{"momo": "github.com/azrtydxb/momo"})
	for _, name := range []string{"data", "keys", "rows", "out", "in", "q", "sel", "res", "ctx", "err", "m", "id", "w", "r", "h", "s", "sets", "upd", "ins"} {
		got := a.Alias("example.com/"+name, name)
		if got == name {
			t.Errorf("alias %q is a local of the generated code", name)
		}
	}
	if got := a.Alias("example.com/example", "example"); got != "example" {
		t.Errorf("an ordinary package name changed: %q", got)
	}
	if again := a.Alias("example.com/data", "data"); again == "data" {
		t.Errorf("the second request for a path changed to %q", again)
	}
}
