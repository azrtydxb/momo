// SPDX-License-Identifier: Apache-2.0

package migrate

// These names let the external tests (package migrate_test) reach internals
// that are unexported in the package proper.
var (
	Discover         = discover
	ParseDirectives  = readDirectives
	Chain            = snapshotChain
	RefuseGoRollback = refuseGoRollback
)
