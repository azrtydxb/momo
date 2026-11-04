# M1: PostgreSQL + pgvector, end to end

Status: open
Created: 2026-11-04

## Goal

A Go developer can define models as ordinary structs, generate code, query and write PostgreSQL with typed builders and explicit SQL, run versioned migrations with drift detection, use relations, and store and search pgvector embeddings (including hybrid search), with hooks, OTel and momotest available. Every M1 test passes in CI against PostgreSQL with pgvector.

Epics, in execution order: `momo-01-foundation`, `momo-09-observability-testing`, `momo-02-generator`, `momo-03-drivers-postgresql`, `momo-04-migrations-core-postgresql`, `momo-05-writes-queries`, `momo-06-relations`, `momo-07-vectors-pgvector`.
