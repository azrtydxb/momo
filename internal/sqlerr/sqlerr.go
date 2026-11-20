// SPDX-License-Identifier: Apache-2.0

// Package sqlerr recognises driver errors by their message. It imports only the
// standard library.
package sqlerr

import "strings"

// IsNoSuchTable reports whether err is SQLite's "no such table" error for the
// named table. SQLite has no SQLSTATE; its message names the table, and SQLite
// may spell it main.<name>. Only the named table counts, not any other table a
// statement named.
func IsNoSuchTable(err error, name string) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no such table: "+name) || strings.Contains(msg, "no such table: main."+name)
}
