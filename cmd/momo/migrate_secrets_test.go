// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// say and errf strip every registered secret, including a password that is not
// part of any URL in the message.
func TestCLISayAndErrfStripRegisteredSecrets(t *testing.T) {
	var out, errOut bytes.Buffer
	l := &cli{out: &out, errOut: &errOut, secrets: []string{"s3cretOnlyInSecrets"}}
	l.say("value %s here", "s3cretOnlyInSecrets")
	l.errf(3, "auth failed for %s", "s3cretOnlyInSecrets")
	for name, got := range map[string]string{"say": out.String(), "errf": errOut.String()} {
		if strings.Contains(got, "s3cretOnlyInSecrets") || !strings.Contains(got, "***") {
			t.Fatalf("%s must replace the registered secret with ***, got %q", name, got)
		}
	}
}

// noteURL registers the URL and its password, so a later message that carries
// only the password is redacted.
func TestCLINoteURLRegistersPassword(t *testing.T) {
	var errOut bytes.Buffer
	l := &cli{out: &bytes.Buffer{}, errOut: &errOut, env: noEnv}
	l.noteURL("postgres://alice:hunter2pw@db.example.com:5432/app?sslmode=require")
	l.errf(3, "FATAL: password authentication failed (tried %s)", "hunter2pw")
	if strings.Contains(errOut.String(), "hunter2pw") {
		t.Fatalf("the password registered by noteURL leaked: %q", errOut.String())
	}
}

// A URL that cannot be parsed (a bad port) still carries its password into the
// driver's error. Through openEngineAt the output must report the failure and
// hide the password.
func TestCLIMalformedURLWithPasswordIsRedacted(t *testing.T) {
	dir := t.TempDir()
	cfg := yamlPath(t, dir, "migrate:\n  dir:\n    postgres: "+filepath.ToSlash(dir)+"/m\n")
	bad := "postgres://bob:p4ssw0rdbad@127.0.0.1:notaport/none?sslmode=disable"
	var out, errOut bytes.Buffer
	code := runMigrate(&out, &errOut, []string{"--config", cfg, "status", "--url", bad}, noEnv)
	all := out.String() + errOut.String()
	if code == 0 || !strings.Contains(all, "momo:") {
		t.Fatalf("an unparsable URL must fail with a reported error, exit %d: %q", code, all)
	}
	if strings.Contains(all, "p4ssw0rdbad") || strings.Contains(all, bad) {
		t.Fatalf("output leaks the DSN: %s", all)
	}
}
