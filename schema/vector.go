// SPDX-License-Identifier: Apache-2.0

package schema

import "errors"

// ErrInvalidVectorIndex is wrapped by the vector index validation errors of
// the migration engines.
var ErrInvalidVectorIndex = errors.New("schema: invalid vector index")
