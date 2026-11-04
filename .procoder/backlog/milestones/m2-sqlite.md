# M2: SQLite + sqlite-vec parity

Status: open
Created: 2026-11-04

## Goal

The same models, queries and migrations run on SQLite (mattn + sqlite-vec, and pure-Go modernc without vectors). Adding SQLite to testopen.Each turns every parameterized test on for SQLite, and they pass, including the Windows CI job.

Epics, in execution order: `momo-03-drivers-sqlite`, `momo-04-migrations-sqlite`, `momo-07-vectors-sqlite-vec`, `momo-09-observability-testing-sqlite`.
