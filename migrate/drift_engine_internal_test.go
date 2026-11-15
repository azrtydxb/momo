// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"testing"

	"github.com/azrtydxb/momo/schema"
)

// Catches: a live MyISAM or Aria table reported as Changed against a snapshot,
// which never records an engine.
func TestFlattenIgnoresTableEngine(t *testing.T) {
	mk := func(engine string) *schema.Schema {
		return &schema.Schema{Dialect: "mysql", Tables: []schema.Table{{
			Name: "articles", Engine: engine,
			Columns:    []schema.Column{{Name: "id", Type: "bigint"}},
			PrimaryKey: []string{"id"},
		}}}
	}
	a, b := flatten(mk(""), false), flatten(mk("MyISAM"), false)
	for k, o := range a {
		if b[k].body != o.body {
			t.Fatalf("object %s differs:\n%s\n%s", k, o.body, b[k].body)
		}
	}
	if len(a) != len(b) {
		t.Fatalf("object count differs: %d vs %d", len(a), len(b))
	}
}
