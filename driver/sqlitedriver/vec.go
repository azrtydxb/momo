// SPDX-License-Identifier: Apache-2.0

package sqlitedriver

import (
	"fmt"

	"github.com/azrtydxb/momo"
)

// SQLiteVecVersion is the sqlite-vec release this module embeds, as
// vec_version() reports it. Open checks it, so a go.mod that pulls in another
// release of the bindings fails at Open instead of changing which distance
// functions exist. Vector columns are BLOB columns with a length CHECK; see
// momo.SQLiteVectorType and momo.SQLiteBitsType.
const SQLiteVecVersion = "v0.1.6"

// checkVecVersion returns an error unless got, the result of SELECT
// vec_version(), is SQLiteVecVersion.
func checkVecVersion(got string) error {
	if got == SQLiteVecVersion {
		return nil
	}
	return &momo.Error{Kind: momo.ErrUnsupported, Op: "open", Err: fmt.Errorf("sqlitedriver: vec_version() is %q, this module is built for sqlite-vec %s", got, SQLiteVecVersion)}
}
