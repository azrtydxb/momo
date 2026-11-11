// SPDX-License-Identifier: Apache-2.0

package naming

import "testing"

func TestTitleASCIIEmpty(t *testing.T) {
	if got := titleASCII(""); got != "" {
		t.Fatalf("titleASCII(\"\") = %q", got)
	}
	if got := titleASCII("uRL"); got != "Url" {
		t.Fatalf("titleASCII = %q", got)
	}
}

func TestInitialismReplacerSkipsEmpty(t *testing.T) {
	r := newInitialismReplacer([]string{"", "API"})
	if got := r.Replace("userAPIKey"); got != "userApiKey" {
		t.Fatalf("Replace = %q", got)
	}
}

// Catches: an initialism followed by a plural s split into letters
// (RoleIDs as role_i_ds), a plural run that swallows the word before it
// (HTTPIDs), or a lone s after a word that is not an initialism being joined.
func TestToSnakeInitialisms(t *testing.T) {
	cases := map[string]string{
		"RoleIDs":         "role_ids",
		"RestrictRoleIDs": "restrict_role_ids",
		"IDs":             "ids",
		"URLs":            "urls",
		"UUIDs":           "uuids",
		"APIs":            "apis",
		"UserAPIs":        "user_apis",
		"APIKeys":         "api_keys",
		"HTTPServer":      "http_server",
		"XMLIDs":          "xml_ids",
		"ID":              "id",
		"UserID":          "user_id",
		"UserIDsList":     "user_ids_list",
		"UserIDs2":        "user_ids2",
		"Ids":             "ids",
		"IDsomething":     "i_dsomething", // the s starts a lower-case word: unchanged from before
		"OAuthIDs":        "o_auth_ids",
		"ABs":             "a_bs", // AB is not an initialism of the list
	}
	for in, want := range cases {
		if got := ToSnake(in); got != want {
			t.Errorf("ToSnake(%q) = %q, want %q", in, got, want)
		}
	}
}
