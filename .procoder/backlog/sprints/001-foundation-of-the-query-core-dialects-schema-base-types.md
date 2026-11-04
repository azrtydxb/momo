# Sprint 1: Foundation of the query core

Status: active
Created: 2026-11-04

## Goal

Plan momo-01 is fully delivered and its gate is green. The uncommitted spike becomes the tested foundation every later plan builds on:
- dialects with full capability flags and quoting;
- schema base types with JSON round-trip, format versions, normalization and canonical sort;
- structured errors and sentinels;
- the fake driver and fixtures;
- placeholders, raw SQL and SELECT rendering under golden and fuzz tests;
- keyset pagination, query immutability, cursor closing and the compile-fail harness;
- the `internal/testdb` harness with per-test PostgreSQL schemas;
- CI on free runners, security workflows, release-please and the import-graph guard.

Done means the Task 23 foundation gate passes locally and in CI against PostgreSQL.
