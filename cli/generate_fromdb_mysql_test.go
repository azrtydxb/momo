// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

// Catches: a mysql:// URL passed to go-sql-driver as it is, which cannot parse
// it, a go-sql-driver DSN rewritten, or a URL parameter that would reach the
// server as a session variable (ssl-mode, socket) being accepted.
func TestMySQLDSN(t *testing.T) {
	cases := map[string]string{
		"mysql://u:p%40ss@db.example:3307/app?tls=skip-verify": "u:p@ss@tcp(db.example:3307)/app?tls=skip-verify",
		"mariadb://u@h/app":          "u@tcp(h:3306)/app",
		"u:p@tcp(h:3306)/app":        "u:p@tcp(h:3306)/app",
		"tcp(h:3306)/app":            "tcp(h:3306)/app",
		"unix(/run/mysqld.sock)/app": "unix(/run/mysqld.sock)/app",
	}
	for in, want := range cases {
		got, err := mysqlDSN(in)
		if err != nil {
			t.Errorf("mysqlDSN(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("mysqlDSN(%q) = %q, want %q", in, got, want)
		}
	}
	for _, ok := range []string{"mysql://u:p@h/app?allowCleartextPasswords=true", "mysql://u:p@h/app?serverPubKey=mykey"} {
		if _, err := mysqlDSN(ok); err != nil {
			t.Errorf("mysqlDSN(%q): %v", ok, err)
		}
	}
	for _, in := range []string{"mysql://u:p@h/app?ssl-mode=REQUIRED", "mysql://u:p@h/app?socket=/tmp/s", "mysql://u:pw@h/app?sql_mode=ANSI"} {
		_, err := mysqlDSN(in)
		if err == nil || !strings.Contains(err.Error(), "go-sql-driver DSN") || strings.Contains(err.Error(), ":p@") || strings.Contains(err.Error(), "pw") {
			t.Errorf("mysqlDSN(%q) error = %v, want a refusal that names the parameter and no password", in, err)
		}
	}
}

// Catches: a go-sql-driver DSN read as the name of an environment variable.
func TestLooksLikeMySQLDSN(t *testing.T) {
	for _, in := range []string{"u:p@tcp(h:3306)/app", "tcp(h:3306)/app", "unix(/run/mysqld.sock)/app"} {
		if !looksLikeMySQLDSN(in) {
			t.Errorf("%q is a DSN", in)
		}
	}
	if looksLikeMySQLDSN("MOMO_DATABASE_URL") {
		t.Error("a variable name is not a DSN")
	}
}

func mysqlCLI() (*cli, *bytes.Buffer) {
	var errb bytes.Buffer
	return &cli{out: &bytes.Buffer{}, errOut: &errb, env: func(string) string { return "" }, dialectName: "mysql"}, &errb
}

// Catches: the password of a native DSN or a converted mysql:// URL, and the
// converted DSN itself, not being registered for redaction, and a remote
// server reached with no tls= raising no warning (or a local one raising it).
func TestMySQLRedactionAndTLSWarning(t *testing.T) {
	l, errb := mysqlCLI()
	l.noteURL("u:s3cret@tcp(db.example.com:3306)/app")
	if !slices.Contains(l.secrets, "s3cret") {
		t.Errorf("secrets = %q, want the password of the DSN", l.secrets)
	}
	if !strings.Contains(errb.String(), "db.example.com") || !strings.Contains(errb.String(), "tls") || strings.Contains(errb.String(), "s3cret") {
		t.Errorf("warning = %q, want the host, the tls advice and no password", errb.String())
	}

	l, errb = mysqlCLI()
	l.noteURL("mysql://u:p%40ss@db.example.com/app")
	if !slices.Contains(l.secrets, "p@ss") || !slices.Contains(l.secrets, "u:p@ss@tcp(db.example.com:3306)/app") {
		t.Errorf("secrets = %q, want the decoded password and the converted DSN", l.secrets)
	}
	if !strings.Contains(errb.String(), "tls") {
		t.Errorf("a remote URL with no tls must warn, got %q", errb.String())
	}

	// A short password is covered by the full DSN and URL, not by itself: a
	// bare "pw" would mangle every message with those two letters in it.
	l, _ = mysqlCLI()
	l.noteURL("mysql://u:pw@h.example.com/app?tls=true")
	if slices.Contains(l.secrets, "pw") || !slices.Contains(l.secrets, "u:pw@tcp(h.example.com:3306)/app?tls=true") {
		t.Errorf("secrets = %q, want no bare short password and the converted DSN", l.secrets)
	}
	l, _ = mysqlCLI()
	l.noteURL("u:pass1@tcp(h.example.com:3306)/app?tls=true")
	if !slices.Contains(l.secrets, "pass1") {
		t.Errorf("secrets = %q, want a password of four or more characters", l.secrets)
	}

	for _, quiet := range []string{
		"u:p@tcp(127.0.0.1:3306)/app",
		"u:p@tcp(localhost)/app",
		"u:p@unix(/run/mysqld.sock)/app",
		"u:p@tcp(db.example.com:3306)/app?tls=true",
		"mysql://u:p@db.example.com/app?tls=skip-verify",
	} {
		l, errb = mysqlCLI()
		l.noteURL(quiet)
		if errb.Len() != 0 {
			t.Errorf("%q must not warn, got %q", quiet, errb.String())
		}
	}
}
