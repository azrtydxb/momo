// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"github.com/azrtydxb/momo/dialect"
)

// renderMySQLLock appends the locking clause of a SELECT: the lock mode, the
// OF tables, then the wait option.
//
//	FOR UPDATE            -> FOR UPDATE                (MySQL 8.0+, MariaDB)
//	FOR SHARE, MySQL      -> FOR SHARE                 (MySQL 8.0.1+)
//	FOR SHARE, MariaDB    -> LOCK IN SHARE MODE        (MariaDB 11.8 has no FOR SHARE)
//	OF t1, t2             -> OF t1, t2                 (MySQL only; MariaDB is ErrUnsupported)
//	NOWAIT                -> NOWAIT                    (MySQL 8.0+, MariaDB 10.3+)
//	SKIP LOCKED           -> SKIP LOCKED               (MySQL 8.0+, MariaDB 10.6+, InnoDB only)
func renderMySQLLock(b *Builder, flavor dialect.Flavor, share bool, of []string, opts ...string) {
	if len(of) > 0 && flavor == dialect.FlavorMariaDB {
		b.unsupported("LockOf (FOR UPDATE OF)")
		return
	}
	switch {
	case !share:
		b.WriteString(" FOR UPDATE")
	case flavor == dialect.FlavorMariaDB:
		b.WriteString(" LOCK IN SHARE MODE")
	default:
		b.WriteString(" FOR SHARE")
	}
	for i, t := range of {
		if i == 0 {
			b.WriteString(" OF ")
		} else {
			b.WriteString(", ")
		}
		b.Ident(t)
	}
	for _, o := range opts {
		if o == "" {
			continue
		}
		b.writeByte(' ')
		b.WriteString(o)
	}
}
