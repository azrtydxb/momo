// SPDX-License-Identifier: Apache-2.0

package naming

import (
	"encoding/json"
	"os"
	"testing"
)

type fixture struct {
	ORM       string            `json:"orm"`
	Version   string            `json:"version"`
	Tables    map[string]string `json:"tables"`
	Columns   map[string]string `json:"columns"`
	JoinTable map[string]string `json:"join_tables"`
}

func TestNamingFixturesRecorded(t *testing.T) {
	for _, orm := range []string{"gorm", "bun", "ent"} {
		b, err := os.ReadFile("testdata/" + orm + ".json")
		if err != nil {
			t.Fatal(err)
		}
		var f fixture
		if err := json.Unmarshal(b, &f); err != nil {
			t.Fatalf("%s: %v", orm, err)
		}
		if f.ORM != orm || f.Version == "" || f.Tables["UserProfile"] == "" || len(f.Columns) == 0 || len(f.JoinTable) == 0 {
			t.Fatalf("%s fixture incomplete: %+v", orm, f)
		}
	}
}
