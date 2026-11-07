// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLITLSWarning(t *testing.T) {
	warn := []string{
		"postgres://app:x@db.example.com/app?sslmode=disable",
		"postgresql://app:x@10.0.0.5:5432/app?sslmode=disable",
		"postgres://app:x@localhost,db.example.com/app?sslmode=disable",
		"postgres://app:x@db.example.com/app?sslmode=prefer",
		"postgres://app:x@db.example.com/app?sslmode=allow",
		// sslmode unset: libpq defaults to prefer, which falls back to plaintext.
		"postgres://app:x@db.example.com/app",
		// key=value strings.
		"host=db.example.com user=app password=x dbname=app sslmode=disable",
		"host=db.example.com user=app password=x dbname=app",
		"host='db.example.com' user=app password='p w' dbname=app sslmode='prefer'",
		"user=app host=localhost,db.example.com sslmode=disable",
	}
	for _, u := range warn {
		w := TLSWarning(u, noEnv)
		if !strings.Contains(w, "TLS") || !strings.Contains(w, "verify-full") {
			t.Errorf("want TLS warning recommending verify-full for %q, got %q", u, w)
		}
		if strings.Contains(w, "sslmode=require") {
			t.Errorf("warning must not recommend require: %q", w)
		}
		if strings.Contains(w, "x@") || strings.Contains(w, "/app") || strings.Contains(w, "password") {
			t.Errorf("warning must not echo the connection string: %q", w)
		}
	}
	quiet := []string{
		"postgres://app:x@localhost/app?sslmode=disable",
		"postgres://app:x@127.0.0.1:5432/app?sslmode=disable",
		"postgres://app:x@[::1]:5432/app?sslmode=disable",
		"postgres://app:x@/app?host=/var/run/postgresql&sslmode=disable",
		"postgres://app:x@%2Fvar%2Frun%2Fpostgresql/app?sslmode=disable",
		"postgres://app:x@localhost/app",
		"postgres:///app",
		"host=/var/run/postgresql dbname=app",
		"host=localhost dbname=app",
		"dbname=app user=app",
		"postgres://app:x@db.example.com/app?sslmode=require",
		"postgres://app:x@db.example.com/app?sslmode=verify-ca",
		"postgres://app:x@db.example.com/app?sslmode=verify-full",
		"host=db.example.com sslmode=verify-full",
		"not a url \x7f",
		"host='unterminated",
		"",
	}
	for _, u := range quiet {
		if w := TLSWarning(u, noEnv); w != "" {
			t.Errorf("no warning wanted for %q, got %q", u, w)
		}
	}
}

func TestCLITLSWarningUsesLibpqEnvironment(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	// PGSSLMODE supplies the mode when the string has none.
	if w := TLSWarning("postgres://app:x@db.example.com/app", env(map[string]string{"PGSSLMODE": "disable"})); !strings.Contains(w, "sslmode=disable") {
		t.Errorf("PGSSLMODE=disable must warn: %q", w)
	}
	if w := TLSWarning("host=db.example.com dbname=app", env(map[string]string{"PGSSLMODE": "verify-full"})); w != "" {
		t.Errorf("PGSSLMODE=verify-full must be quiet: %q", w)
	}
	// An explicit sslmode wins over PGSSLMODE.
	if w := TLSWarning("host=db.example.com sslmode=verify-full", env(map[string]string{"PGSSLMODE": "disable"})); w != "" {
		t.Errorf("an explicit sslmode wins over PGSSLMODE: %q", w)
	}
	// PGHOST supplies the host when the string has none.
	if w := TLSWarning("dbname=app", env(map[string]string{"PGHOST": "db.example.com"})); !strings.Contains(w, "db.example.com") {
		t.Errorf("PGHOST must supply the remote host: %q", w)
	}
	if w := TLSWarning("dbname=app", env(map[string]string{"PGHOST": "/var/run/postgresql"})); w != "" {
		t.Errorf("a socket PGHOST is local: %q", w)
	}
}

func TestMigrateWarnsOnRemoteDisabledTLS(t *testing.T) {
	dir := t.TempDir()
	cfg := yamlPath(t, dir, "migrate:\n  dir:\n    postgres: "+filepath.ToSlash(dir)+"/m\n")
	run := func(url string) string {
		var out, errOut bytes.Buffer
		runMigrate(&out, &errOut, []string{"--config", cfg, "status", "--url", url}, noEnv)
		return out.String() + errOut.String()
	}
	// 192.0.2.1 is TEST-NET (remote); connect_timeout keeps the run short.
	all := run("postgres://alice:hunter2secret@192.0.2.1:1/none?sslmode=disable&connect_timeout=1")
	if !strings.Contains(all, "TLS") {
		t.Fatalf("remote sslmode=disable must warn: %s", all)
	}
	if strings.Contains(all, "hunter2secret") {
		t.Fatalf("warning leaks the password: %s", all)
	}
	if all := run("postgres://alice:hunter2secret@127.0.0.1:1/none?sslmode=disable&connect_timeout=1"); strings.Contains(all, "TLS") {
		t.Fatalf("local sslmode=disable must not warn: %s", all)
	}
}
