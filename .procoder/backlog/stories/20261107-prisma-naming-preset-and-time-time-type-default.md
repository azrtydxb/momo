# Prisma naming preset and time.Time type default

Status: open
Created: 2026-11-07
Epic: momo-11-adoption
Sprint: -

## Description

GitHub issue #28. A team porting a Prisma-backed Node service to Go (NovaCRM,
280 DateTime columns) needs momo models to match the database Prisma created,
without a column tag on every field and a type tag on every timestamp. This adds
a `naming: prisma` preset (quoted PascalCase tables, camelCase columns, and the
constraint and index names Prisma uses) and a per-project default PostgreSQL
type for `time.Time` in momo.yaml (`types: {time.Time: timestamp(3)}`). Done
means a hand-written Prisma DDL database diffs clean against the models.

## Acceptance criteria

- [ ] `naming.ByName("prisma")` returns a preset whose table name is the model name and whose column names are camelCase (unit test).
- [ ] Constraint and index names come out as `<Table>_pkey`, `<Table>_<col>_fkey`, `<Table>_<col>_key`, `<Table>_<col>_idx` for a prisma-named model (schema test).
- [ ] momo.yaml accepts `types: {time.Time: ...}`, rejects other keys, and the value reaches the generator (config test).
- [ ] An integration test creates the tables as Prisma would (timestamp(3), constraint names) and `migrate diff` against the prisma-preset models reports no change, while models without the type default do report one.
- [ ] docs/ai-guide.md and docs/TYPE_MAPPING.md document the preset and the type default.
- [ ] CI is green on the final commit including integration jobs.

## Evidence

<!-- Filled at close time. -->
