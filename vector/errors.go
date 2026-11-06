// SPDX-License-Identifier: Apache-2.0

package vector

import "errors"

// ErrInvalidVector reports a vector value that fails validation, such as a
// dimension mismatch or a non-finite component. The root package re-exports
// it as momo.ErrInvalidVector.
var ErrInvalidVector = errors.New("momo/vector: invalid vector")

// ErrDialect reports a vector type the target dialect cannot store or query.
var ErrDialect = errors.New("momo/vector: type not supported by dialect")
