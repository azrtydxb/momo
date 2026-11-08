// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Catches: Text writing a file that lacks the generated header, overwriting a
// handwritten file, or accepting two different contents for one path.
func TestWriterTextRefusals(t *testing.T) {
	root := t.TempDir()
	good := "%% " + Header()[len("// "):] + "\nerDiagram\n"
	w := NewWriter(WriterOptions{ModuleRoot: root})

	p := filepath.Join(root, "out", "schema.mmd")
	if err := w.Text(p, []byte(good)); err != nil {
		t.Fatal(err)
	}
	if err := w.Text(p, []byte(good)); err != nil {
		t.Fatalf("identical repeat: %v", err)
	}
	if err := w.Text(p, []byte(good+"x\n")); !errors.Is(err, ErrCollision) {
		t.Fatalf("different repeat: %v", err)
	}
	if err := w.Text(filepath.Join(root, "no_header.txt"), []byte("hello\n")); err == nil {
		t.Fatal("data without the generated header was accepted")
	}
	if err := w.Text("relative.txt", []byte(good)); err == nil {
		t.Fatal("relative path accepted")
	}
	if err := w.Text(filepath.Join(filepath.Dir(root), "outside.mmd"), []byte(good)); err == nil {
		t.Fatal("path outside the module root accepted")
	}

	hand := filepath.Join(root, "hand.mmd")
	if err := os.WriteFile(hand, []byte("erDiagram\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := w.Text(hand, []byte(good)); !errors.Is(err, ErrHandwritten) {
		t.Fatalf("handwritten: %v", err)
	}

	changed, err := w.Commit()
	if err != nil || len(changed) != 1 || changed[0] != p {
		t.Fatalf("commit = %v, %v", changed, err)
	}
	got, err := os.ReadFile(p) //nolint:gosec // test path
	if err != nil || string(got) != good {
		t.Fatalf("content = %q, %v", got, err)
	}

	// A second run over the generated file is allowed and finds it current.
	w2 := NewWriter(WriterOptions{ModuleRoot: root})
	if err := w2.Text(p, []byte(good)); err != nil {
		t.Fatal(err)
	}
	if changed, err = w2.Commit(); err != nil || len(changed) != 0 {
		t.Fatalf("recommit = %v, %v", changed, err)
	}
}

// Catches: Scaffold overwriting a file the user has edited.
func TestWriterScaffoldNeverOverwrites(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "handlers.go")
	w := NewWriter(WriterOptions{ModuleRoot: root})
	if err := w.Scaffold(p, []byte("package x\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("package x // mine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	w = NewWriter(WriterOptions{ModuleRoot: root})
	if err := w.Scaffold(p, []byte("package x\n")); err != nil {
		t.Fatal(err)
	}
	changed, err := w.Commit()
	if err != nil || len(changed) != 0 {
		t.Fatalf("commit = %v, %v", changed, err)
	}
	got, err := os.ReadFile(p) //nolint:gosec // test path
	if err != nil || string(got) != "package x // mine\n" {
		t.Fatalf("scaffold overwrote the user's file: %q, %v", got, err)
	}
}

// Catches: Scaffold writing outside the module root through "..", through a
// symlink that leaves the root, or replacing an existing directory.
func TestWriterScaffoldConfinement(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	outside := filepath.Join(base, "outside")
	for _, d := range []string{root, outside, filepath.Join(root, "dir")} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	w := NewWriter(WriterOptions{ModuleRoot: root})

	if err := w.Scaffold(filepath.Join(root, "..", "outside", "x.go"), []byte("package x\n")); err == nil {
		t.Fatal("a path that leaves the root through .. was accepted")
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := w.Scaffold(filepath.Join(root, "link", "x.go"), []byte("package x\n")); err == nil {
		t.Fatal("a path through a symlink that leaves the root was accepted")
	}
	if err := w.Scaffold(filepath.Join(root, "link"), []byte("package x\n")); err == nil {
		t.Fatal("a symlink that leaves the root was accepted as the scaffold path")
	}
	// An existing directory is skipped, not replaced.
	if err := w.Scaffold(filepath.Join(root, "dir"), []byte("package x\n")); err != nil {
		t.Fatalf("existing directory: %v", err)
	}
	if changed, err := w.Commit(); err != nil || len(changed) != 0 {
		t.Fatalf("commit = %v, %v", changed, err)
	}
	if fi, err := os.Stat(filepath.Join(root, "dir")); err != nil || !fi.IsDir() {
		t.Fatalf("directory was replaced: %v", err)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("wrote outside the root: %v", entries)
	}
}
