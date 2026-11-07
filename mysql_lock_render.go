// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"strings"

	"github.com/azrtydxb/momo/dialect"
)

// renderMySQLLock appends the locking clause of a SELECT.
//
//	FOR UPDATE            -> FOR UPDATE                (MySQL 8.0+, MariaDB)
//	FOR SHARE, MySQL      -> FOR SHARE                 (MySQL 8.0.1+)
//	FOR SHARE, MariaDB    -> LOCK IN SHARE MODE        (MariaDB 11.8 has no FOR SHARE)
//	NOWAIT                -> NOWAIT                    (MySQL 8.0+, MariaDB 10.3+)
//	SKIP LOCKED           -> SKIP LOCKED               (MySQL 8.0+, MariaDB 10.6+, InnoDB only)
func renderMySQLLock(b *Builder, flavor dialect.Flavor, share bool, opts ...string) {
	switch {
	case !share:
		b.WriteString(" FOR UPDATE")
	case flavor == dialect.FlavorMariaDB:
		b.WriteString(" LOCK IN SHARE MODE")
	default:
		b.WriteString(" FOR SHARE")
	}
	for _, o := range opts {
		b.writeByte(' ')
		b.WriteString(o)
	}
}

// renderLock appends a lock clause held as "FOR UPDATE [opt]" or
// "FOR SHARE [opt]", in the syntax of the builder's dialect.
func renderLock(b *Builder, lock string) {
	if b.d.Name != dialect.MySQL {
		b.writeByte(' ')
		b.WriteString(lock)
		return
	}
	share := strings.HasPrefix(lock, "FOR SHARE")
	rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(lock, "FOR SHARE"), "FOR UPDATE"))
	if rest == "" {
		renderMySQLLock(b, b.d.Flavor, share)
		return
	}
	renderMySQLLock(b, b.d.Flavor, share, rest)
}
