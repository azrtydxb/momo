// SPDX-License-Identifier: Apache-2.0

package snake

import "testing"

// Catches: the shared word list answering differently for a word and its
// plural, or for another letter case.
func TestInitialism(t *testing.T) {
	for in, want := range map[string]string{
		"id": "ID", "ID": "ID", "ids": "IDs", "Urls": "URLs", "uuids": "UUIDs",
		"api": "API", "http": "HTTP", "https": "HTTPS",
	} {
		if got, ok := Initialism(in); !ok || got != want {
			t.Errorf("Initialism(%q) = %q, %v, want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "s", "user", "users", "key", "keys", "uses", "ss"} {
		if got, ok := Initialism(in); ok {
			t.Errorf("Initialism(%q) = %q, want none", in, got)
		}
	}
}
