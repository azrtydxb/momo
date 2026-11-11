// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"testing"

	"github.com/azrtydxb/momo/gen/naming"
)

// Catches: the --from-db field name and naming.ToSnake using different word
// lists, so a column does not come back as itself: role_ids must give RoleIDs
// and RoleIDs must give role_ids, and the same for the singular initialisms.
func TestFieldNameRoundTripsThroughToSnake(t *testing.T) {
	for column, field := range map[string]string{
		"role_ids":          "RoleIDs",
		"restrict_role_ids": "RestrictRoleIDs",
		"urls":              "URLs",
		"uuids":             "UUIDs",
		"api_keys":          "APIKeys",
		"http_server":       "HTTPServer",
		"id":                "ID",
		"user_id":           "UserID",
		"apis":              "APIs",
		"user_ids_list":     "UserIDsList",
	} {
		if got := pascal(splitWords(column), "F"); got != field {
			t.Errorf("pascal(%q) = %q, want %q", column, got, field)
		}
		if got := naming.ToSnake(field); got != column {
			t.Errorf("ToSnake(%q) = %q, want %q", field, got, column)
		}
	}
}
