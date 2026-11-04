# momo-03 Task 11: Read-replica routing with a unit of work and AllowStaleReads (S-40)

Status: open
Created: 2026-11-04
Epic: momo-03-drivers-postgresql
Sprint: -

## Description

Implements plan momo-03-drivers Task 11: Read-replica routing with a unit of work and AllowStaleReads (S-40) (`.procoder/plans/momo-03-drivers.md:1333`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-40. Files: `replica.go`, `replica_test.go`, `db.go`, `driver_optional.go`.

## Acceptance criteria

- [ ] `TestReadReplicaRouting` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestReplicaUnsupportedWithoutReplicas` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-40] `TestReadReplicaRouting`: queries on the read handle reach the replica driver (driver-identity hook), queries inside a transaction never do; fails if a read inside a transaction is routed to the replica.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

