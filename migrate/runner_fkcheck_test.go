// SPDX-License-Identifier: Apache-2.0

package migrate

import "testing"

func TestForeignKeyCheckStatement(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		hint string
		want bool
		stmt string // the text that is run
	}{
		{"plain", "PRAGMA foreign_key_check", "", true, "PRAGMA foreign_key_check"},
		{"semicolon and case", "pragma Foreign_Key_Check ;\n", "", true, "pragma Foreign_Key_Check"},
		{"leading line comment", "-- rebuild step\nPRAGMA foreign_key_check;", "", true, "PRAGMA foreign_key_check"},
		{"leading block comment", "/* check */  \n PRAGMA foreign_key_check", "", true, "PRAGMA foreign_key_check"},
		{"several comments", "-- a\n-- b\n/* c */\nPRAGMA foreign_key_check", "", true, "PRAGMA foreign_key_check"},
		{"schema", "PRAGMA main.foreign_key_check", "", true, "PRAGMA main.foreign_key_check"},
		{"quoted schema", `PRAGMA "main".foreign_key_check`, "", true, `PRAGMA "main".foreign_key_check`},
		{"table argument", "PRAGMA foreign_key_check(items)", "", true, "PRAGMA foreign_key_check(items)"},
		{"quoted table argument", `PRAGMA foreign_key_check( "items" );`, "", true, `PRAGMA foreign_key_check( "items" )`},
		{"schema and table", "PRAGMA main.foreign_key_check('items')", "", true, "PRAGMA main.foreign_key_check('items')"},
		{"hint without text match", "SELECT 1", "fk-check", true, ""},
		{"another pragma", "PRAGMA foreign_keys = OFF", "", false, ""},
		{"integrity check", "PRAGMA integrity_check", "", false, ""},
		{"trailing statement", "PRAGMA foreign_key_check; DROP TABLE t", "", false, ""},
		{"inside a comment only", "-- PRAGMA foreign_key_check", "", false, ""},
		{"unterminated comment", "/* PRAGMA foreign_key_check", "", false, ""},
		{"set form", "PRAGMA foreign_key_check = 1", "", false, ""},
		{"plain statement", "CREATE TABLE foreign_key_check (a int)", "", false, ""},
	}
	for _, c := range cases {
		if got := isForeignKeyCheck(Statement{SQL: c.sql, Hint: c.hint}); got != c.want {
			t.Errorf("%s: isForeignKeyCheck = %v, want %v", c.name, got, c.want)
		}
		if c.want && c.hint == "" {
			if stmt, ok := foreignKeyCheckSQL(c.sql); !ok || stmt != c.stmt {
				t.Errorf("%s: foreignKeyCheckSQL = %q, %v; want %q", c.name, stmt, ok, c.stmt)
			}
		}
	}
}
