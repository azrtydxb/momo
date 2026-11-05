// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

const pastedURL = "postgres://u:topsecret@h/db"

// A pasted URL that lands in a flag position must never echo its password:
// the flag package prints parse errors verbatim unless the CLI takes over.
func TestMigrateFlagParseErrorsAreRedacted(t *testing.T) {
	dir := t.TempDir()
	cfg := yamlPath(t, dir, "migrate:\n  dir:\n    postgres: "+filepath.ToSlash(dir)+"/m\n")
	cases := []struct {
		name string
		args []string
	}{
		{"url as rename-table value", []string{"--config", cfg, "diff", "x", "--rename-table", pastedURL}},
		{"undefined flag carrying a url", []string{"--config", cfg, "plan", "--bogus=" + pastedURL}},
		{"top-level flag that is a url", []string{"--" + pastedURL, "plan"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := runMigrate(&out, &errOut, c.args, noEnv)
			if code != 2 {
				t.Fatalf("exit %d, want 2: %s", code, errOut.String())
			}
			if all := out.String() + errOut.String(); strings.Contains(all, "topsecret") {
				t.Fatalf("password leaked: %s", all)
			}
			if !strings.Contains(errOut.String(), "Usage") {
				t.Fatalf("usage text missing: %s", errOut.String())
			}
		})
	}
}

func TestGenerateFlagParseErrorIsRedacted(t *testing.T) {
	for _, arg := range []string{"--bogus=" + pastedURL, "--check=" + pastedURL} {
		var out, errOut bytes.Buffer
		code := runGenerate([]string{arg}, &out, &errOut)
		if code != 2 {
			t.Fatalf("%s: exit %d, want 2", arg, code)
		}
		if all := out.String() + errOut.String(); strings.Contains(all, "topsecret") {
			t.Fatalf("%s: password leaked: %s", arg, all)
		}
		if !strings.Contains(errOut.String(), "Usage") {
			t.Fatalf("%s: usage missing: %s", arg, errOut.String())
		}
	}
}

func TestRunUnknownCommandIsRedacted(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run([]string{pastedURL}, &out, &errOut); code != 2 {
		t.Fatalf("exit %d", code)
	}
	if strings.Contains(out.String()+errOut.String(), "topsecret") {
		t.Fatalf("leaked: %s", errOut.String())
	}
	errOut.Reset()
	Run([]string{"help", pastedURL}, &out, &errOut)
	if strings.Contains(out.String()+errOut.String(), "topsecret") {
		t.Fatalf("help leaked: %s", errOut.String())
	}
}

func TestSubcommandHelpStillExitsZero(t *testing.T) {
	dir := t.TempDir()
	cfg := yamlPath(t, dir, "migrate:\n  dir:\n    postgres: "+filepath.ToSlash(dir)+"/m\n")
	var out, errOut bytes.Buffer
	if code := runMigrate(&out, &errOut, []string{"--config", cfg, "plan", "-h"}, noEnv); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out.String()+errOut.String(), "Usage") {
		t.Fatal("no usage printed")
	}
	out.Reset()
	errOut.Reset()
	if code := runGenerate([]string{"-h"}, &out, &errOut); code != 0 || out.Len() == 0 {
		t.Fatalf("generate -h: exit %d", code)
	}
}
