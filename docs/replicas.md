# Read replicas

`WithReplicas` registers read-replica drivers on the primary `DB`.
The primary handle never reads from them. `DB.Replica` with an index returns a handle
whose reads may go to that replica; it returns `ErrUnsupported` when that replica
is not configured, and replicas do not chain.

## What goes to a replica

A statement goes to the replica only when it is provably a plain read: it
starts with SELECT, WITH, VALUES or EXPLAIN and contains no INSERT, UPDATE,
DELETE, MERGE, INTO or COPY, no row lock clause (FOR UPDATE, FOR SHARE, FOR NO
KEY UPDATE, FOR KEY SHARE), no sequence or session-state function (nextval,
setval, lastval, set_config, advisory locks, pg_notify), and no dollar-quoted
body. Matching ignores string literals, quoted identifiers and comments. A
statement the classifier cannot read with confidence counts as a write.

These never go to a replica, whatever the handle:

- every write (Exec), and every transaction, including reads inside one;
- INSERT, UPDATE and DELETE with RETURNING;
- locking reads, such as FOR UPDATE SKIP LOCKED;
- raw SQL that writes, such as a WITH that contains DML, or a SELECT of
  nextval.

A false positive only costs a read on the primary; the classifier never sends
a write to a read-only server (`TestReplicaRoutesWritesAndLocksToPrimary`).

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
