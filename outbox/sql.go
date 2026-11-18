// SPDX-License-Identifier: Apache-2.0

package outbox

import "github.com/azrtydxb/momo/dialect"

// statements holds the SQL of one outbox table on one dialect. The table name
// is validated by New to be a plain identifier and is quoted here; every value
// is bound as an argument.
//
// claim and lease differ by dialect. PostgreSQL and SQLite claim in the one
// statement claim, which updates and returns the rows. MySQL and MariaDB have
// no UPDATE ... RETURNING: claim only selects (with FOR UPDATE SKIP LOCKED) and
// lease, completed with a parenthesized id list, updates what claim returned,
// inside one short transaction.
type statements struct {
	insert    string
	claim     string
	lease     string
	published string
	retry     string
	dead      string
	release   string
	cleanup   string
	requeue   string
}

// dueClause is the WHERE clause of a claim over the table aliased e, with q the
// quoted table and k the quoted key column: the event is pending, due, and has
// no earlier unpublished event of its key. A NULL key never matches, so an
// event without a key is unordered. It binds one argument, the current time.
func dueClause(q, k string) string {
	return `e.published_at IS NULL AND e.dead_at IS NULL AND e.available_at <= ?
	AND NOT EXISTS (
		SELECT 1 FROM ` + q + ` p
		WHERE p.` + k + ` = e.` + k + ` AND p.id < e.id AND p.published_at IS NULL
	)`
}

func newStatements(d dialect.Name, table string) statements {
	q, k := `"`+table+`"`, `"key"`
	if d == dialect.MySQL {
		q, k = "`"+table+"`", "`key`"
	}
	due := dueClause(q, k)
	var s statements

	s.insert = `INSERT INTO ` + q + ` (topic, ` + k + `, payload, headers, created_at, available_at, attempts)
VALUES (?, ?, ?, ?, ?, ?, 0)`
	if d != dialect.MySQL {
		s.insert += ` RETURNING id`
	}

	switch d {
	case dialect.Postgres:
		// Arguments: now, batch size, lease end. The row locks last for this one
		// statement; SKIP LOCKED leaves the rows another relay is claiming to it.
		s.claim = `WITH due AS (
	SELECT e.id FROM ` + q + ` e
	WHERE ` + due + `
	ORDER BY e.id
	LIMIT ?
	FOR UPDATE SKIP LOCKED
)
UPDATE ` + q + ` SET attempts = ` + q + `.attempts + 1, available_at = ?
FROM due WHERE ` + q + `.id = due.id
RETURNING ` + q + `.id, ` + q + `.topic, ` + q + `.` + k + `, ` + q + `.payload, ` + q + `.headers, ` + q + `.attempts`
	case dialect.SQLite:
		// Arguments: lease end, now, batch size. SQLite has one writer, so the
		// single statement is already exclusive and needs no row locks.
		s.claim = `UPDATE ` + q + ` SET attempts = attempts + 1, available_at = ?
WHERE id IN (
	SELECT e.id FROM ` + q + ` e
	WHERE ` + due + `
	ORDER BY e.id
	LIMIT ?
)
RETURNING id, topic, ` + k + `, payload, headers, attempts`
	case dialect.MySQL:
		// Arguments of claim: now, batch size. It runs in a READ COMMITTED
		// transaction with lease, whose arguments are the lease end and the ids.
		s.claim = `SELECT e.id, e.topic, e.` + k + `, e.payload, e.headers, e.attempts FROM ` + q + ` e
WHERE ` + due + `
ORDER BY e.id
LIMIT ?
FOR UPDATE SKIP LOCKED`
		s.lease = `UPDATE ` + q + ` SET attempts = attempts + 1, available_at = ? WHERE id IN `
	}

	s.published = `UPDATE ` + q + ` SET published_at = ?, last_error = NULL WHERE id = ?`
	s.retry = `UPDATE ` + q + ` SET available_at = ?, last_error = ? WHERE id = ?`
	s.dead = `UPDATE ` + q + ` SET dead_at = ?, last_error = ? WHERE id = ?`
	s.release = `UPDATE ` + q + ` SET available_at = ?, attempts = attempts - 1 WHERE id = ? AND published_at IS NULL`
	if d == dialect.MySQL {
		// MySQL cannot select from the table it deletes from in a subquery, nor
		// LIMIT one, but takes ORDER BY and LIMIT on a single-table DELETE.
		s.cleanup = `DELETE FROM ` + q + ` WHERE published_at IS NOT NULL AND published_at < ? ORDER BY id LIMIT ?`
	} else {
		s.cleanup = `DELETE FROM ` + q + ` WHERE id IN (
	SELECT id FROM ` + q + ` WHERE published_at IS NOT NULL AND published_at < ? ORDER BY id LIMIT ?
)`
	}
	// The caller appends the parenthesized list of ids.
	s.requeue = `UPDATE ` + q + ` SET dead_at = NULL, attempts = 0, last_error = NULL, available_at = ? WHERE dead_at IS NOT NULL AND id IN `
	return s
}
