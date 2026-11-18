# Read replicas

`WithReplicas` registers read-replica drivers on the primary `DB`.
The primary handle never reads from them. `DB.Replica` with an index returns a handle
whose reads may go to that replica; it returns `ErrUnsupported` when that replica
is not configured, and replicas do not chain.

## What goes to a replica

A statement goes to the replica only when it is clearly a plain read: one
statement that starts with SELECT, WITH, VALUES or TABLE, or with EXPLAIN, its
options (ANALYZE, VERBOSE, a parenthesised list) and then one of those. It must
contain none of these words outside literals, quoted identifiers and
comments: INSERT, UPDATE, DELETE, MERGE, INTO, COPY, CREATE, DROP, ALTER,
TRUNCATE, REFRESH, EXECUTE, CALL, DO, LISTEN, NOTIFY, LOCK, DECLARE, GRANT,
REVOKE, VACUUM, ANALYZE, CLUSTER, REINDEX, COMMENT, SECURITY, IMPORT; no row
lock clause (FOR UPDATE, FOR SHARE, FOR NO KEY UPDATE, FOR KEY SHARE); none of
the side-effect functions nextval, setval, lastval, set_config, pg_notify,
txid_current, pg_terminate_backend, pg_cancel_backend, lo_import, lo_export,
lo_unlink, dblink_exec, pg_reload_conf, pg_switch_wal,
pg_logical_emit_message, or the advisory-lock functions; and no
dollar-quoted body. A semicolon followed by more text (a second statement) is
refused. Anything else goes to the primary. The list is wide on purpose: a
false positive, for example an unquoted column called comment, only costs a
read on the primary.

These never go to a replica, whatever the handle:

- every write (Exec), and every transaction, including reads inside one;
- INSERT, UPDATE and DELETE with RETURNING;
- locking reads, such as FOR UPDATE SKIP LOCKED;
- raw SQL that writes, such as a WITH that contains DML, or a SELECT of
  nextval.

One case cannot be detected: a SELECT that calls a user-defined function which
writes looks like a plain read. Run such a call on the primary handle or inside
a transaction. The classifier is covered by
`TestReplicaRoutesWritesAndLocksToPrimary`.

A statement that goes to the primary because it is not a plain read counts as
a write of the unit of work, so the reads that follow it in the same context
see its effects.

## Combining with other options

`DB.Replica` returns `ErrUnsupported` on a DB built with `WithImplicitTx`:
that option runs every statement in its own primary transaction, so a replica
could never be used. A read on a replica runs outside any transaction, so
`WithTxSetup` does not apply to it. Do not rely on a setting that
`WithTxSetup` installs, such as a row-level-security tenant, for replica reads;
use the primary handle for tenant-scoped reads.

## Reading your own writes

Replicas lag. Pass a context from `NewUnitOfWork` through one request: the
first write made through any handle with that context is recorded, and from
then on reads of a Replica handle in that context go to the primary. Wrap a
context with `AllowStaleReads` to send reads back to the replica after a write
when stale data is acceptable (`TestReadReplicaRouting`).

## When the replica is down

If the replica fails with a connection-class error when a read starts
(connection refused or reset, EOF, a network error, SQLSTATE class 08, 57P01 to
57P03 or 53300), that read runs on the primary
(`TestReplicaFailureFallsBackToPrimary`). Other errors, such as a syntax
error, are returned as they are, and so are errors raised after rows have
started to arrive: there is no retry in the middle of a result set. A cancelled
or expired caller context is never retried.
