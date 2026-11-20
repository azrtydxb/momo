// SPDX-License-Identifier: Apache-2.0

package redact

import (
	"strings"
	"testing"
)

func TestRedactURLs(t *testing.T) {
	got := RedactURLs("dial postgres://u:pw@h/db?password=x and password=hunter2 end", "hunter2")
	for _, bad := range []string{"u:pw", "password=x", "hunter2"} {
		if strings.Contains(got, bad) {
			t.Fatalf("%q still contains %q", got, bad)
		}
	}
}

func TestRedactURLsForms(t *testing.T) {
	for in, leaks := range map[string][]string{
		"dial root:hunter2@tcp(db.local:3306)/app?parseTime=true failed": {"hunter2"},
		"dial root:p4ss@unix(/var/run/mysqld.sock)/app":                  {"p4ss"},
		`conn password='it\'s secret' host=h`:                            {"it", "secret"},
		`PGPASSWORD=s3cr3t psql`:                                         {"s3cr3t"},
		`conn PASSWORD="a \"b\" c" x`:                                    {`\"b`},
		"dial postgres://u:pw1@h:5432/db":                                {"pw1"},
		"dial root:p@ss@tcp(db:3306)/app":                                {"p@ss", "ss@"},
	} {
		got := RedactURLs(in)
		for _, l := range leaks {
			if strings.Contains(got, l) {
				t.Errorf("RedactURLs(%q) = %q still contains %q", in, got, l)
			}
		}
	}
	if got := RedactURLs("dial root:hunter2@tcp(db.local:3306)/app"); !strings.Contains(got, "root:***@tcp(db.local:3306)") {
		t.Errorf("only the password may change: %q", got)
	}
	for in, want := range map[string]string{
		"password= host=h":        "password= host=h",
		"SET password = $1":       "SET password = $1",
		"WHERE pwd = ? AND x = 1": "WHERE pwd = ? AND x = 1",
		"password=a=b host=h":     "password=*** host=h",
	} {
		if got := RedactURLs(in); got != want {
			t.Errorf("RedactURLs(%q) = %q, want %q", in, got, want)
		}
	}
	if got := RedactURLs("relation foo: not found at (1:2)"); got != "relation foo: not found at (1:2)" {
		t.Errorf("ordinary text changed: %q", got)
	}
}
