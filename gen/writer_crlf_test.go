// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func crlf(s string) []byte { return []byte(strings.ReplaceAll(s, "\n", "\r\n")) }

// A CRLF checkout of the current output must be up to date: check lists
// nothing, plain generate leaves the bytes (and mtime) alone, and orphan
// detection still sees CRLF generated files.
func TestWriterTreatsCRLFAsUpToDate(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "sub", "a_gen.go")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, crlf(testHeader), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	orphan := filepath.Join(root, "sub", "gone_gen.go")
	if err := os.WriteFile(orphan, crlf(testHeader), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, check := range []bool{true, false} {
		w := NewWriter(WriterOptions{ModuleRoot: root, Check: check})
		if err := w.File(p, []byte(testHeader)); err != nil {
			t.Fatal(err)
		}
		changed, err := w.Commit()
		if err != nil || len(changed) != 0 {
			t.Fatalf("check=%v: CRLF file reported changed: %v %v", check, changed, err)
		}
		orphans, err := w.Orphans([]string{filepath.Dir(p)})
		if err != nil || !reflect.DeepEqual(orphans, []string{orphan}) {
			t.Fatalf("check=%v: orphans %v %v", check, orphans, err)
		}
	}
	got, err := os.ReadFile(p)
	if err != nil || string(got) != string(crlf(testHeader)) {
		t.Fatalf("CRLF file was rewritten: %q %v", got, err)
	}
	if fi, _ := os.Stat(p); !fi.ModTime().Equal(old) {
		t.Fatal("CRLF file mtime changed")
	}
}

// A CRLF file whose content really differs is still stale, and rewriting it
// emits LF.
func TestWriterCRLFContentDriftStillStale(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "a_gen.go")
	if err := os.WriteFile(p, crlf(testHeader+"// drift\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := NewWriter(WriterOptions{ModuleRoot: root, Check: true})
	if err := w.File(p, []byte(testHeader)); err != nil {
		t.Fatal(err)
	}
	if changed, err := w.Commit(); err != nil || !reflect.DeepEqual(changed, []string{p}) {
		t.Fatalf("drift not reported: %v %v", changed, err)
	}
	w = NewWriter(WriterOptions{ModuleRoot: root})
	if err := w.File(p, []byte(testHeader)); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Commit(); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if string(got) != testHeader {
		t.Fatalf("rewritten content %q want LF output", got)
	}
}
