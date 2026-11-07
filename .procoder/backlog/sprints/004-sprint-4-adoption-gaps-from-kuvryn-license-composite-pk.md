# Sprint 4: adoption gaps from kuvryn-license - composite-PK foreign keys, serial ids, aliases, INSERT SELECT, typed expressions

Status: active
Created: 2026-11-07

## Goal

An existing PostgreSQL application such as kuvryn-license can describe its live schema in momo models with no hand-edited init migration (composite-key foreign keys, serial ids), and write its store queries without raw SQL fragments: aliased select-list expressions, INSERT ... SELECT, and typed substr, CASE, column arithmetic and current_schema. Closes GitHub issues #13 to #17.
