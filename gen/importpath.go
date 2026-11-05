// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ImportPath returns the slash-separated Go import path of the package in
// pkgDir, given the module path and the module directory (from
// packages.Module). It is an error when pkgDir is outside moduleDir.
func ImportPath(modulePath, moduleDir, pkgDir string) (string, error) {
	rel, err := filepath.Rel(moduleDir, pkgDir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("gen: %s is outside module %s", pkgDir, moduleDir)
	}
	if rel == "." {
		return modulePath, nil
	}
	return modulePath + "/" + filepath.ToSlash(rel), nil
}
