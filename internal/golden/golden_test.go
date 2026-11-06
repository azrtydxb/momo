// SPDX-License-Identifier: Apache-2.0

package golden

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGoldenDiff(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a", "b.golden")

	ok, err := Diff(p, []byte("x"), false)
	if err != nil || ok {
		t.Fatalf("missing file: want (false,nil), got (%v,%v)", ok, err)
	}
	ok, err = Diff(p, []byte("x"), true)
	if err != nil || !ok {
		t.Fatalf("update: want (true,nil), got (%v,%v)", ok, err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "x" {
		t.Fatalf("update wrote %q", b)
	}
	ok, err = Diff(p, []byte("x"), false)
	if err != nil || !ok {
		t.Fatalf("equal: want (true,nil), got (%v,%v)", ok, err)
	}
	ok, err = Diff(p, []byte("y"), false)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("different content reported as match")
	}
}
