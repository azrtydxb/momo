// SPDX-License-Identifier: Apache-2.0

package outbox

// statements holds the SQL of one outbox table. The table name is validated by
// New to be a plain identifier and is quoted here; every value is bound as an
// argument.
type statements struct {
	insert      string
	claimPG     string
	claimSQLite string
	published   string
	retry       string
	dead        string
	release     string
	cleanup     string
	requeue     string
}

// dueClause is the WHERE clause of a claim over the table aliased e, with q the
// quoted table: the event is pending, due, and has no earlier unpublished event
// of its key. A NULL key never matches, so an event without a key is
// unordered. It binds one argument, the current time.
func dueClause(q string) string {
	return `e.published_at IS NULL AND e.dead_at IS NULL AND e.available_at <= ?
	AND NOT EXISTS (
		SELECT 1 FROM ` + q + ` p
		WHERE p."key" = e."key" AND p.id < e.id AND p.published_at IS NULL
	)`
}

func newStatements(table string) statements {
	q := `"` + table + `"`
	due := dueClause(q)
	var s statements

	s.insert = `INSERT INTO ` + q + ` (topic, "key", payload, headers, created_at, available_at, attempts)
VALUES (?, ?, ?, ?, ?, ?, 0) RETURNING id`

	// Arguments: now, batch size, lease end. The row locks last for this one
	// statement; SKIP LOCKED leaves the rows another relay is claiming to it.
	s.claimPG = `WITH due AS (
	SELECT e.id FROM ` + q + ` e
	WHERE ` + due + `
	ORDER BY e.id
	LIMIT ?
	FOR UPDATE SKIP LOCKED
)
UPDATE ` + q + ` SET attempts = ` + q + `.attempts + 1, available_at = ?
FROM due WHERE ` + q + `.id = due.id
RETURNING ` + q + `.id, ` + q + `.topic, ` + q + `."key", ` + q + `.payload, ` + q + `.headers, ` + q + `.attempts`

	// Arguments: lease end, now, batch size. SQLite has one writer, so the
	// single statement is already exclusive and needs no row locks.
	s.claimSQLite = `UPDATE ` + q + ` SET attempts = attempts + 1, available_at = ?
WHERE id IN (
	SELECT e.id FROM ` + q + ` e
	WHERE ` + due + `
	ORDER BY e.id
	LIMIT ?
)
RETURNING id, topic, "key", payload, headers, attempts`

	s.published = `UPDATE ` + q + ` SET published_at = ?, last_error = NULL WHERE id = ?`
	s.retry = `UPDATE ` + q + ` SET available_at = ?, last_error = ? WHERE id = ?`
	s.dead = `UPDATE ` + q + ` SET dead_at = ?, last_error = ? WHERE id = ?`
	s.release = `UPDATE ` + q + ` SET available_at = ?, attempts = attempts - 1 WHERE id = ? AND published_at IS NULL`
	s.cleanup = `DELETE FROM ` + q + ` WHERE id IN (
	SELECT id FROM ` + q + ` WHERE published_at IS NOT NULL AND published_at < ? ORDER BY id LIMIT ?
)`
	// The caller appends the parenthesized list of ids.
	s.requeue = `UPDATE ` + q + ` SET dead_at = NULL, attempts = 0, last_error = NULL, available_at = ? WHERE dead_at IS NOT NULL AND id IN `
	return s
}
