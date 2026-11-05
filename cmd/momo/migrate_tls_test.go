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
	}
	for _, u := range warn {
		w := TLSWarning(u)
		if !strings.Contains(w, "TLS") {
			t.Errorf("want TLS warning for %q, got %q", u, w)
		}
		if strings.Contains(w, "x@") || strings.Contains(w, "/app") {
			t.Errorf("warning must not echo the URL: %q", w)
		}
	}
	quiet := []string{
		"postgres://app:x@localhost/app?sslmode=disable",
		"postgres://app:x@127.0.0.1:5432/app?sslmode=disable",
		"postgres://app:x@[::1]:5432/app?sslmode=disable",
		"postgres://app:x@/app?host=/var/run/postgresql&sslmode=disable",
		"postgres://app:x@%2Fvar%2Frun%2Fpostgresql/app?sslmode=disable",
		"postgres://app:x@db.example.com/app?sslmode=require",
		"postgres://app:x@db.example.com/app",
		"not a url \x7f",
	}
	for _, u := range quiet {
		if w := TLSWarning(u); w != "" {
			t.Errorf("no warning wanted for %q, got %q", u, w)
		}
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
