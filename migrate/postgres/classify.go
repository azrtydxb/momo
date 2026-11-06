// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/azrtydxb/momo"
)

var (
	// extUnavailable matches the server's "extension is not available" (the
	// package is not installed on the server) and "permission denied to create
	// extension" (the role may not install it) messages.
	extUnavailable = regexp.MustCompile(`extension "([^"]+)" is not available|permission denied to create extension "([^"]+)"`)
	// vectorType matches the failure of a pgvector type used before its
	// extension exists.
	vectorType = regexp.MustCompile(`type "(vector|halfvec|sparsevec)" does not exist`)
)

// ClassifyError turns the failures that a missing extension causes into a
// momo.ErrUnsupported error that says which extension is not installed and keeps
// the driver error as its cause: CREATE EXTENSION on a server without the
// package or by a role that may not create it, and a pgvector type used
// without the extension. Every other error is returned unchanged. The
// migration engine applies it to the errors of Apply and AutoMigrate.
func (Dialect) ClassifyError(err error) error {
	if err == nil {
		return nil
	}
	var s interface{ SQLState() string }
	if !errors.As(err, &s) {
		return err
	}
	msg := err.Error()
	ext := ""
	if m := extUnavailable.FindStringSubmatch(msg); m != nil {
		ext = m[1] + m[2]
	} else if m := vectorType.FindStringSubmatch(msg); m != nil && s.SQLState() == "42704" {
		ext = "vector"
	}
	if ext == "" {
		return err
	}
	return &momo.Error{Kind: momo.ErrUnsupported, Op: "migrate", Err: fmt.Errorf("extension %q is not installed (install its server package and run CREATE EXTENSION as a role that may): %w", ext, err)}
}
