// SPDX-License-Identifier: Apache-2.0

package naming

import "testing"

// TestPrismaPreset catches a Prisma identifier rule that drifts: tables keep
// the model name, columns are camelCase with Id for an ID word, and the
// implicit relation table is _AToB in alphabetical order.
func TestPrismaPreset(t *testing.T) {
	n, err := ByName("prisma")
	if err != nil {
		t.Fatal(err)
	}
	for model, want := range map[string]string{"KbChunk": "KbChunk", "Account": "Account"} {
		if got := n.TableName(model); got != want {
			t.Errorf("TableName(%q)=%q want %q", model, got, want)
		}
	}
	for field, want := range map[string]string{
		"CreatedAt": "createdAt", "ID": "id", "AccountID": "accountId", "Email": "email",
		"HTTPServer": "httpServer", "TenantIDNumber": "tenantIdNumber", "Version2": "version2",
	} {
		if got := n.ColumnName(field); got != want {
			t.Errorf("ColumnName(%q)=%q want %q", field, got, want)
		}
	}
	if got := n.JoinTableName("Post", "Category"); got != "_CategoryToPost" {
		t.Errorf("JoinTableName=%q", got)
	}
}
