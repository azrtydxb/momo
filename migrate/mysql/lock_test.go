// SPDX-License-Identifier: Apache-2.0

package mysql

import (
	"strings"
	"testing"
)

func TestLockName(t *testing.T) {
	if got := lockName("momo:app.momo_migrations"); got != "momo:app.momo_migrations" {
		t.Fatalf("a short key must pass through, got %q", got)
	}
	a := lockName("momo:" + strings.Repeat("a", 80) + ".one")
	b := lockName("momo:" + strings.Repeat("a", 80) + ".two")
	if len(a) != maxLockName || len(b) != maxLockName {
		t.Fatalf("a long key must map to %d characters, got %d and %d", maxLockName, len(a), len(b))
	}
	if a == b {
		t.Fatalf("keys that differ only past the limit must keep different lock names, both are %q", a)
	}
	if a != lockName("momo:"+strings.Repeat("a", 80)+".one") {
		t.Fatal("the lock name of a key must be deterministic")
	}
}

func TestHistoryLockKey(t *testing.T) {
	d := Dialect{}
	if got := d.HistoryLockKey("`app`.`momo_migrations`"); got != "momo:app.momo_migrations" {
		t.Fatalf("HistoryLockKey = %q", got)
	}
	if got := d.HistoryLockKey("`we``ird`.`momo_migrations`"); got != "momo:we`ird.momo_migrations" {
		t.Fatalf("HistoryLockKey with a quote = %q", got)
	}
}

func TestMySQLDDLIsNotTransactional(t *testing.T) {
	if (Dialect{}).Dialect().TransactionalDDL {
		t.Fatal("MySQL DDL commits implicitly; the dialect must not report transactional DDL")
	}
}
