// SPDX-License-Identifier: Apache-2.0

// Package schemautil holds small helpers shared by the schema model, its
// migration engines and the code generator. It is a leaf: it imports only the
// standard library and the dialect package, never schema or momo.
package schemautil

// Int64 returns a pointer to v, for the optional numeric fields of
// schema.Sequence.
func Int64(v int64) *int64 { return &v }
