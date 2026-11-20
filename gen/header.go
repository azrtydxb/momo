// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"strconv"

	"github.com/azrtydxb/momo/internal/genutil"
)

// Header returns the single generated-file header line. It matches Go's
// convention `^// Code generated .* DO NOT EDIT\.$`; every emitter uses it.
// It carries the format number, which decides compatibility, and no momo
// release version, so upgrading momo does not rewrite generated files.
func Header() string {
	return genutil.HeaderPrefix + "(format " + strconv.Itoa(FormatVersion) + "). DO NOT EDIT."
}
