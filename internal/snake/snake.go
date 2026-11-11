// SPDX-License-Identifier: Apache-2.0

// Package snake is the Go-style snake_case conversion and the initialism word
// list behind it. The core package, gen/naming and gen/fromdb share it, so a
// column name and a Go field name map to each other the same way everywhere.
package snake

import (
	"strings"
	"unicode"
)

// ToSnake converts a Go identifier to snake_case, keeping acronyms together.
// A plural s after an initialism of the word list belongs to it: RoleIDs is
// role_ids and URLs is urls, not role_i_ds and ur_ls.
func ToSnake(s string) string {
	rs := []rune(s)
	noSplit, forceSplit := pluralInitialisms(rs)
	var b strings.Builder
	for i, r := range rs {
		if unicode.IsUpper(r) && i > 0 {
			prev := rs[i-1]
			split := unicode.IsLower(prev) || unicode.IsDigit(prev) ||
				(unicode.IsUpper(prev) && i+1 < len(rs) && unicode.IsLower(rs[i+1]))
			if (split && !noSplit[i]) || forceSplit[i] {
				b.WriteByte('_')
			}
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// pluralInitialisms finds the upper-case runs that end in an initialism of the
// word list followed by a lone plural s (the "IDs" of RoleIDs). noSplit marks
// the last letter of such a run, which the usual rule would split off before
// the s, and forceSplit the first letter of the initialism when the run has
// letters before it (the "ID" of HTTPIDs). The s must end the name or be
// followed by something that is not a lower-case letter, so "IDsx" is not a
// plural.
func pluralInitialisms(rs []rune) (noSplit, forceSplit []bool) {
	noSplit = make([]bool, len(rs))
	forceSplit = make([]bool, len(rs))
	for j := 0; j < len(rs); j++ {
		if !unicode.IsUpper(rs[j]) {
			continue
		}
		e := j
		for e+1 < len(rs) && unicode.IsUpper(rs[e+1]) {
			e++
		}
		if e+1 < len(rs) && rs[e+1] == 's' && (e+2 >= len(rs) || !unicode.IsLower(rs[e+2])) {
			if n := initialismSuffix(rs[j : e+1]); n > 0 {
				noSplit[e] = true
				if k := e + 1 - n; k > j {
					forceSplit[k] = true
				}
			}
		}
		j = e
	}
	return noSplit, forceSplit
}

// goInitialisms is the word list of the Go-style snake_case and of the
// --from-db field names: a word in it is written in capitals inside a Go name
// (UserID, not UserId), and a plural s after it belongs to it (IDs, URLs).
var goInitialisms = map[string]bool{
	"ACL": true, "API": true, "ASCII": true, "CPU": true, "CSS": true,
	"DNS": true, "EOF": true, "GUID": true, "HTML": true, "HTTP": true,
	"HTTPS": true, "ID": true, "IP": true, "JSON": true, "LHS": true,
	"QPS": true, "RAM": true, "RHS": true, "RPC": true, "SLA": true,
	"SMTP": true, "SQL": true, "SSH": true, "TCP": true, "TLS": true,
	"TTL": true, "UDP": true, "UI": true, "UID": true, "URI": true,
	"URL": true, "UUID": true, "VM": true, "XML": true, "XMPP": true,
	"XSRF": true, "XSS": true,
}

// maxInitialism is the length of the longest word in goInitialisms.
const maxInitialism = 5

// Initialism reports whether word, in any letter case, is an initialism
// of the word list (id, URL) or its plural (ids, Urls), and returns its Go
// spelling: ID, URL, IDs, URLs. It is the list ToSnake and the --from-db
// field names share, so a name round-trips between a column and a Go field.
func Initialism(word string) (string, bool) {
	up := strings.ToUpper(word)
	if goInitialisms[up] {
		return up, true
	}
	if n := len(up); n > 1 && up[n-1] == 'S' && goInitialisms[up[:n-1]] {
		return up[:n-1] + "s", true
	}
	return "", false
}

// initialismSuffix returns the length of the longest word of the list that
// the upper-case run ends with, or 0.
func initialismSuffix(run []rune) int {
	for n := min(len(run), maxInitialism); n > 0; n-- {
		if goInitialisms[string(run[len(run)-n:])] {
			return n
		}
	}
	return 0
}
