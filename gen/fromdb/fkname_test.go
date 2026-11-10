// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"strings"
	"testing"
)

// Catches: a foreign key with a name other than the derived one (PostgreSQL's
// _fkey1 on a partitioned table) being left out of the models and the snapshot.
func TestGenerateFromDBForeignKeyName(t *testing.T) {
	s := sampleSchema()
	s.Tables[0].ForeignKeys[0].Name = "orders_user_id_fkey1"
	res, files := generate(t, s, Config{})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	mustContain(t, "orders model", files["orders"], "fk_name:orders_user_id_fkey1")
	for _, f := range res.Findings {
		if strings.Contains(f.Object, "orders_user_id_fkey1") {
			t.Errorf("the foreign key is declarable, no finding expected: %v", f)
		}
	}
	for _, tb := range res.Snapshot.Tables {
		if tb.Name == "orders" && (len(tb.ForeignKeys) != 1 || tb.ForeignKeys[0].Name != "orders_user_id_fkey1") {
			t.Errorf("snapshot: %+v", tb.ForeignKeys)
		}
	}
}
