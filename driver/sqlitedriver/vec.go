// SPDX-License-Identifier: Apache-2.0

//go:build cgo && !windows

package sqlitedriver

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/azrtydxb/momo"
)

// VecVersion is the sqlite-vec release this module is built and tested
// against, as vec_version() reports it. Open accepts that release and any later
// patch release of the same minor version (v0.1.x at or above this), so a go.mod
// that resolves the bindings to a newer patch release keeps working; another
// minor or major version fails at Open, because the set of distance functions
// may differ. The test TestSQLiteVecLoaded compares vec_version() with this
// constant exactly, so a change of the bindings version in go.mod is noticed.
// Vector columns are BLOB columns with a length CHECK, such as
// BLOB CHECK (length("emb") = 12) for three float32 elements. A build without
// cgo declares the constant too, as the empty string.
const VecVersion = "v0.1.6"

// SQLiteVecVersion is the old name of VecVersion.
//
// Deprecated: use VecVersion. It is removed in the next release.
const SQLiteVecVersion = VecVersion

// parseVecVersion reads "vMAJOR.MINOR.PATCH". A pre-release or build suffix is
// not a release and does not parse.
func parseVecVersion(v string) (major, minor, patch int, ok bool) {
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if !strings.HasPrefix(v, "v") || len(parts) != 3 {
		return 0, 0, 0, false
	}
	n := [3]int{}
	for i, p := range parts {
		x, err := strconv.Atoi(p)
		if err != nil || x < 0 {
			return 0, 0, 0, false
		}
		n[i] = x
	}
	return n[0], n[1], n[2], true
}

// checkVecVersion returns an error unless got, the result of SELECT
// vec_version(), is VecVersion or a later patch release of its minor
// version.
func checkVecVersion(got string) error {
	wantMajor, wantMinor, wantPatch, _ := parseVecVersion(VecVersion)
	major, minor, patch, ok := parseVecVersion(got)
	if ok && major == wantMajor && minor == wantMinor && patch >= wantPatch {
		return nil
	}
	return &momo.Error{Kind: momo.ErrUnsupported, Op: "open", Err: fmt.Errorf("sqlitedriver: vec_version() is %q, this module needs sqlite-vec %s or a later patch release of v%d.%d", got, VecVersion, wantMajor, wantMinor)}
}
