# Sprint 8: fix the Kuvryn Orbit port issues #73 to #79 and the open follow-ups

Status: active
Created: 2026-11-10

## Goal

Kuvryn Orbit can port its remaining modules without workarounds:
- **`--from-db`:** it can model a subset of tables, writes only the functions those tables use, and keeps every database default.
- **Raw reads:** they scan into plain structs of any width.
- **UUIDs:** `google/uuid` works without a type tag.
- **Writes:** a `RETURNING` write has a one-row helper.
- **Registry:** its SQL matches what runs.

The flaky `TestApplyCancelledStillUnlocks` gets a named cause and a fix, and the generator simplify cuts land. The sprint ships as v0.7.0.
