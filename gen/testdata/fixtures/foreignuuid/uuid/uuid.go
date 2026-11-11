// SPDX-License-Identifier: Apache-2.0

// Package uuid is a stand-in for another UUID package: it is named uuid, like
// github.com/google/uuid, but it is not that package.
package uuid

// UUID is another package's 16-byte identifier.
type UUID [16]byte
