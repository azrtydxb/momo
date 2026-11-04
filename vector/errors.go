// SPDX-License-Identifier: Apache-2.0

// Package vector holds the vector value types.
package vector

import "errors"

// ErrInvalidVector reports a vector value that fails validation, such as a
// dimension mismatch or a non-finite component. The root package re-exports
// it as momo.ErrInvalidVector.
var ErrInvalidVector = errors.New("vector: invalid vector")
