// SPDX-License-Identifier: Apache-2.0

// Package migrate runs schema migrations.
package migrate

import "errors"

// Sentinel errors returned by the migration engine; match with errors.Is.
var (
	// ErrChecksumMismatch: an applied migration no longer matches its recorded checksum.
	ErrChecksumMismatch = errors.New("migrate: checksum mismatch")
	// ErrDirty: a previous migration failed part-way and left the database dirty.
	ErrDirty = errors.New("migrate: database has a dirty migration; run momo migrate resolve after manual repair")
	// ErrLocked: another process holds the migration lock.
	ErrLocked = errors.New("migrate: migration lock is held by another process")
	// ErrDestructive: the migration drops or rewrites data and was not authorized.
	ErrDestructive = errors.New("migrate: destructive change requires --allow-destructive or a -- momo:destructive directive")
	// ErrBranchConflict: two migration branches modify the same history point.
	ErrBranchConflict = errors.New("migrate: two migrations share the same parent snapshot")
	// ErrOutOfOrder: a pending migration is older than the latest applied one.
	ErrOutOfOrder = errors.New("migrate: a pending migration is older than the latest applied migration")
	// ErrStagedRequired: a NOT NULL column without default on a populated table needs a staged change.
	ErrStagedRequired = errors.New("migrate: NOT NULL column without default on a populated table requires a staged change (--stage)")
	// ErrNoDown: the migration has no authored down file.
	ErrNoDown = errors.New("migrate: migration has no authored down file")
	// ErrNoSnapshot: no snapshot exists for the requested version.
	ErrNoSnapshot = errors.New("migrate: no snapshot found for the requested version")
	// ErrBaselineMismatch: the live database does not match the snapshot a baseline was asked to record.
	ErrBaselineMismatch = errors.New("migrate: the live database does not match the snapshot")
	// ErrCanonicalizePrivilege: canonicalizing declared view, function, trigger or policy text needs a privilege the connected role lacks.
	ErrCanonicalizePrivilege = errors.New("migrate: canonicalizing the snapshot needs a privilege the connected role lacks")
	// ErrRebuildBlocked: a table rebuild is blocked by an object referencing a dropped column.
	ErrRebuildBlocked = errors.New("migrate: table rebuild blocked by an object that references a dropped column")
)
