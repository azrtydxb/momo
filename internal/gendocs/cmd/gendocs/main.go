// SPDX-License-Identifier: Apache-2.0

// Command gendocs rewrites the generated blocks of the documentation. It is
// started by the go:generate directive in package gendocs.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/azrtydxb/momo/internal/gendocs"
)

func main() {
	root, err := moduleRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "gendocs:", err)
		os.Exit(1)
	}
	if err := gendocs.Generate(root); err != nil {
		fmt.Fprintln(os.Stderr, "gendocs:", err)
		os.Exit(1)
	}
}

// moduleRoot walks up from the working directory to the directory whose
// go.mod declares the momo module.
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		b, err := os.ReadFile(filepath.Join(dir, "go.mod")) // #nosec G304 -- walks up from the working directory
		if err == nil && strings.HasPrefix(string(b), "module github.com/azrtydxb/momo\n") {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no go.mod of github.com/azrtydxb/momo above the working directory")
		}
		dir = parent
	}
}
