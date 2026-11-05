// SPDX-License-Identifier: Apache-2.0

// Package migrate runs schema migrations.
package migrate

import "errors"

// Sentinel errors returned by the migration engine; match with errors.Is.
var (
	// ErrChecksumMismatch: an applied migration no longer matches its recorded checksum.
	ErrChecksumMismatch = errors.New("migrate: checksum mismatch")
	// ErrDirty: a previous migration failed part-way and left the database dirty.
	ErrDirty = errors.New("migrate: database is dirty")
	// ErrLocked: another process holds the migration lock.
	ErrLocked = errors.New("migrate: lock held by another process")
	// ErrDestructive: the migration drops or rewrites data and was not authorized.
	ErrDestructive = errors.New("migrate: destructive change not authorized")
	// ErrBranchConflict: two migration branches modify the same history point.
	ErrBranchConflict = errors.New("migrate: migration branches conflict")
)
