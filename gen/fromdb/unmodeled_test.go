// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

// unmodeledObjectsSchema has a modeled table "keep" and a table "audit_log"
// that skip or only leaves out, as inspection does: audit_log is absent from
// Tables while its trigger, policy and functions remain. "audit_log" is not
// the first model here, but with no model at all its objects used to fall back
// to the first model.
func unmodeledObjectsSchema() *schema.Schema {
	fn := func(name, ret, body string) schema.Function {
		return schema.Function{Name: name, Returns: ret, Language: "plpgsql", Volatility: "VOLATILE", Body: body}
	}
	idCol := schema.Column{Name: "id", Type: "bigint", Identity: "by_default"}
	return &schema.Schema{
		FormatVersion: schema.FormatVersion,
		Dialect:       dialect.Postgres,
		Tables:        []schema.Table{{Name: "keep", Columns: []schema.Column{idCol}, PrimaryKey: []string{"id"}}},
		Functions: []schema.Function{
			fn("audit_touch", "trigger", "BEGIN RETURN NEW; END;"),
			fn("audit_can_read", "boolean", "BEGIN RETURN true; END;"),
		},
		Triggers: []schema.Trigger{{Table: "audit_log", Name: "audit_log_touch", Timing: "BEFORE", Events: []string{"UPDATE"}, ForEachRow: true, Function: "audit_touch()"}},
		Policies: []schema.Policy{{Table: "audit_log", Name: "audit_log_read", Command: "SELECT", Permissive: true, Using: "audit_can_read()"}},
	}
}

// Catches: the policies and triggers of a table left out by introspect.skip or
// introspect.only landing on the first model (issue 108), along with the
// functions only they use. introspect.functions: all still writes every function.
func TestObjectsOfUnmodeledTablesAreNotEmitted(t *testing.T) {
	for name, cfg := range map[string]Config{
		"skip": {Skip: []string{"audit_log"}},
		"only": {Only: []string{"keep"}},
	} {
		res, files := generate(t, unmodeledObjectsSchema(), cfg)
		if res.Failed() {
			t.Fatalf("%s: findings: %v", name, res.Findings)
		}
		for _, w := range []string{"audit_log_touch", "audit_log_read", "audit_touch", "audit_can_read", "c.Trigger", "c.Policy", "c.Function"} {
			if strings.Contains(files["keep"], w) {
				t.Errorf("%s: model of keep contains %q:\n%s", name, w, files["keep"])
			}
		}
		if len(res.Snapshot.Triggers) != 0 || len(res.Snapshot.Policies) != 0 || len(res.Snapshot.Functions) != 0 {
			t.Errorf("%s: snapshot keeps objects of an unmodeled table: %+v", name, res.Snapshot)
		}
	}
	cfg := Config{Skip: []string{"audit_log"}, Functions: "all"}
	_, files := generate(t, unmodeledObjectsSchema(), cfg)
	mustContain(t, "functions all", files["keep"], `"audit_touch"`, `"audit_can_read"`)
	if strings.Contains(files["keep"], "audit_log_touch") {
		t.Errorf("functions: all must not bring back the trigger:\n%s", files["keep"])
	}
}
