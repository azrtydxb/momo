# fromdb: objects of unmodeled tables land on the first model

Status: open
Created: 2026-11-11
Epic: -
Sprint: 009-fix-every-open-github-issue-86-to-108-the-kuvryn-orbit-port
Type: bug
Severity: s2

## Description

GitHub issue #108. Reproduce: run `momo generate --from-db` against a database
where table `audit_log` has an RLS policy and a trigger, with `introspect.skip:
[audit_log]` (or `introspect.only` that leaves it out).

Observed: the policy, the trigger and the functions only they use are written
as `c.Policy`, `c.Trigger` and `c.Function` on the alphabetically first model
and into the init snapshot, so every migration that adds RLS to an unmodeled
table changes that file. Expected: an object that belongs to a table with no
model gets no output; `introspect.functions: all` still writes every function.

Root cause: inspection leaves a skipped or only-excluded table out of the
schema but keeps its triggers and policies. `dropObjectsOf` only dropped the
objects of tables that were in the table list, so these never matched, and
`globals` and `functions` fell back to the first model for a table with no
model.

## Acceptance criteria

- [ ] a regression test pins the fix: red before the change, green after
- [ ] with `skip` and with `only`, no trigger, policy or function of an unmodeled table appears in any model or in the snapshot
- [ ] `introspect.functions: all` still writes every function, but not the trigger
- [ ] docs/upgrading.md has a 0.8.0 note

## Evidence

Red run: pending. Green run: pending.
