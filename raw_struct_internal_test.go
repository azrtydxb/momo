// SPDX-License-Identifier: Apache-2.0

package momo

import "testing"

// Catches: a snake_case rule that splits acronyms, digits or single letters
// differently from the documented one.
func TestSnakeCase(t *testing.T) {
	for in, want := range map[string]string{
		"ID":         "id",
		"UserID":     "user_id",
		"Title":      "title",
		"HTTPServer": "http_server",
		"CreatedAt":  "created_at",
		"Title2":     "title2",
		"Line2Text":  "line2_text",
		"A":          "a",
		"URL":        "url",
		"OAuthToken": "o_auth_token",
	} {
		if got := snakeCase(in); got != want {
			t.Errorf("snakeCase(%q) = %q, want %q", in, got, want)
		}
	}
}
