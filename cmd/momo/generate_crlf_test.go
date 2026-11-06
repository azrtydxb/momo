// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustRel(t *testing.T, base, p string) string {
	t.Helper()
	r, err := filepath.Rel(base, p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// TestGenerateCheckCRLFCheckout converts every generated file to CRLF, as a
// core.autocrlf checkout does, and expects --check to pass and generate to
// leave the files untouched; real drift stays stale.
func TestGenerateCheckCRLFCheckout(t *testing.T) {
	root := copyModule(t, exampleSrc)
	chdir(t, root)
	var out, errb bytes.Buffer
	if code := Run([]string{"generate"}, &out, &errb); code != 0 {
		t.Fatalf("generate: exit %d\n%s", code, errb.String())
	}
	var files []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && filepath.Base(p) == "momo_gen.go" {
			files = append(files, p)
		}
		return err
	})
	if err != nil || len(files) == 0 {
		t.Fatalf("no generated files: %v", err)
	}
	crlfBytes := map[string][]byte{}
	for _, p := range files {
		b, err := os.ReadFile(p) //nolint:gosec // test path
		if err != nil {
			t.Fatal(err)
		}
		b = bytes.ReplaceAll(b, []byte("\n"), []byte("\r\n"))
		crlfBytes[p] = b
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	out.Reset()
	errb.Reset()
	if code := Run([]string{"generate", "--check"}, &out, &errb); code != 0 || strings.Contains(out.String(), "stale") {
		t.Fatalf("check on CRLF tree: exit %d\n%s%s", code, out.String(), errb.String())
	}
	if code := Run([]string{"generate"}, &out, &errb); code != 0 {
		t.Fatalf("generate on CRLF tree: exit %d\n%s", code, errb.String())
	}
	for p, want := range crlfBytes {
		if got, _ := os.ReadFile(p); !bytes.Equal(got, want) { //nolint:gosec // test path
			t.Fatalf("%s was rewritten", p)
		}
	}
	drift := append(append([]byte(nil), crlfBytes[files[0]]...), []byte("// drift\r\n")...)
	if err := os.WriteFile(files[0], drift, 0o600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if code := Run([]string{"generate", "--check"}, &out, &errb); code != 1 || !strings.Contains(out.String(), "stale: "+filepath.ToSlash(mustRel(t, root, files[0]))) {
		t.Fatalf("drift not stale: exit %d\n%s", code, out.String())
	}
}
