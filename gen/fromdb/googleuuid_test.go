// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

func uuidSchema() *schema.Schema {
	return &schema.Schema{
		FormatVersion: schema.FormatVersion,
		Dialect:       dialect.Postgres,
		Tables: []schema.Table{{
			Name: "members",
			Columns: []schema.Column{
				{Name: "id", Type: "uuid"},
				{Name: "owner_id", Type: "uuid", Nullable: true},
				{Name: "team_ids", Type: "uuid[]"},
			},
			PrimaryKey: []string{"id"},
		}},
	}
}

// Catches: introspect.uuid: google that still writes types.UUID, writes a type
// tag, or forgets the google/uuid import.
func TestGenerateFromDBGoogleUUID(t *testing.T) {
	res, files := generate(t, uuidSchema(), Config{UUID: "google"})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	src := files["members"]
	mustMatch(t, "members", src,
		`ID\s+uuid\.UUID\s`,
		`OwnerID\s+\*uuid\.UUID\s`,
		`TeamIDs\s+\[\]uuid\.UUID\s`,
		`"github.com/google/uuid"`,
	)
	for _, bad := range []string{"types.UUID", "type:uuid"} {
		if strings.Contains(src, bad) {
			t.Errorf("members must not contain %q:\n%s", bad, src)
		}
	}
}

// Catches: the default changing for users who do not set the switch.
func TestGenerateFromDBDefaultUUIDIsMomo(t *testing.T) {
	res, files := generate(t, uuidSchema(), Config{})
	if res.Failed() {
		t.Fatalf("findings: %v", res.Findings)
	}
	mustMatch(t, "members", files["members"], `ID\s+types\.UUID\s`, `TeamIDs\s+\[\]types\.UUID\s`)
	if strings.Contains(files["members"], "google/uuid") {
		t.Errorf("the default must not import google/uuid:\n%s", files["members"])
	}
}

// Catches: a typo in introspect.uuid that silently falls back to the default.
func TestConfigValidateUUID(t *testing.T) {
	for _, v := range []string{"", "momo", "google"} {
		if err := (&Config{UUID: v}).Validate(); err != nil {
			t.Errorf("uuid %q: %v", v, err)
		}
	}
	err := (&Config{UUID: "gofrs"}).Validate()
	if err == nil || !strings.Contains(err.Error(), "introspect.uuid") {
		t.Errorf("uuid gofrs: err = %v, want one naming introspect.uuid", err)
	}
}
