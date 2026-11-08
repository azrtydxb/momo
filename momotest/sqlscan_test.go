// SPDX-License-Identifier: Apache-2.0

package momotest

import (
	"strings"
	"testing"
)

// squash collapses runs of white space, so a case states what survives
// without counting the spaces that replace removed text.
func squash(b []byte) string { return strings.Join(strings.Fields(string(b)), " ") }

func TestStripSQL(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"plain", "CREATE TABLE a (x int);", "CREATE TABLE a (x int);"},
		{"line comment", "a -- create schema x\nb", "a b"},
		{"block comment", "a /* create schema x */ b", "a b"},
		{"nested block comment", "/* a /* b */ c */ x", "x"},
		{"unterminated block comment", "x /* create schema y", "x"},
		{"string", "a 'create schema x' b", "a b"},
		{"doubled quote", "'it''s' x", "x"},
		{"unterminated string", "x 'create schema y", "x"},
		{"line comment marker in a string", "a '-- not a comment' , b", "a , b"},
		{"block comment marker in a string", "a '/* not a comment' , b", "a , b"},
		{"E string with an escaped quote", `a E'it\'s' b`, "a b"},
		{"E string, trailing backslash", `a E'x\`, "a"},
		{"lowercase e string", `a e'it\'s' b`, "a b"},
		{"plain string keeps a backslash literal", `a 'x\' b`, "a b"},
		{"identifier ending in e is not an E string", "name'x' y", "name y"},
		{"unicode string", `a U&'d\0061t' b`, "a U& b"},
		{"quoted identifier is kept", `"a--b" x`, `"a--b" x`},
		{"quoted identifier with a doubled quote", `"a""--b" x`, `"a""--b" x`},
		{"parameter is not a dollar quote", "x = $1 and y = $2", "x = $1 and y = $2"},
		{"multibyte next to quotes", "é'create schema'é", "é é"},
		{"multibyte in an identifier before a quote", "aé'x'", "aé"},
		{"dollar body is scanned as code", "a $$ create schema x $$ b", "a create schema x b"},
		{"dollar body with a tag", "a $t$ x $t$ b", "a x b"},
		{"dollar body comment is removed", "a $$ x -- create schema y\n z $$ b", "a x z b"},
		{"dollar body string is removed", "a $$ x 'create schema y' z $$ b", "a x z b"},
		{"apostrophe in a dollar body keeps the raw body", "$$don't$$ create schema x", "don't create schema x"},
		{"text after a dollar body is not swallowed", "select $$it's$$; create schema y;", "select it's ; create schema y;"},
		{"unterminated dollar body", "a $$ x", "a x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := squash(stripSQL([]byte(tc.in))); got != tc.want {
				t.Errorf("stripSQL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
