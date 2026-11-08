// SPDX-License-Identifier: Apache-2.0

//go:build cgo && !windows

package sqlitedriver

import (
	"errors"
	"testing"
)

func TestLoadExtensionProbeClassifiesErrors(t *testing.T) {
	if err := checkLoadExtensionError(errors.New("not authorized")); err != nil {
		t.Fatalf("disabled loading reported as enabled: %v", err)
	}
	if err := checkLoadExtensionError(errors.New("x.so: cannot open shared object file: No such file or directory")); err == nil {
		t.Fatal("a missing-file error must not count as disabled loading")
	}
	if err := checkLoadExtensionError(errors.New("no such function: load_extension")); err != nil {
		t.Fatalf("a SQLite built without extension loading reported as enabled: %v", err)
	}
	if err := checkLoadExtensionError(errors.New("no such function: other")); err == nil {
		t.Fatal("an unrelated missing function must not count as disabled loading")
	}
	if err := checkLoadExtensionError(nil); err == nil {
		t.Fatal("success must count as enabled loading")
	}
}
