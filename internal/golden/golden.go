// SPDX-License-Identifier: Apache-2.0

// Package golden compares generated output with golden files.
package golden

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Diff compares got with the golden file at path. When update is true it
// writes got to path (creating parent directories) and reports a match. When
// false it reports whether the file's bytes equal got; a missing file is
// (false, nil).
func Diff(path string, got []byte, update bool) (match bool, err error) {
	if update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { // #nosec G301 -- golden files are committed test fixtures, not secrets; world-readable like any checked-in directory
			return false, err
		}
		if err := os.WriteFile(path, got, 0o644); err != nil { // #nosec G306 -- golden files are committed test fixtures, not secrets; 0644 like any checked-in file
			return false, err
		}
		return true, nil
	}
	want, err := os.ReadFile(path) // #nosec G304 -- path is a golden-file location chosen by the calling test, never untrusted input
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return bytes.Equal(want, got), nil
}
