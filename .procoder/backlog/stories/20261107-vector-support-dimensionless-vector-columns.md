# Vector: support dimensionless vector columns

Status: open
Created: 2026-11-07
Epic: momo-07-vectors-pgvector
Sprint: 005-sprint-5-security-and-maintainability-hardening

## Description

GitHub issue #29, found by the NovaCRM port. A pgvector column declared without `dim` is accepted by the code (gen/schema.go emits plain `vector`, vector_column.go treats a zero Dim as unchecked) while docs/TYPE_MAPPING.md and docs/ai-guide.md say it is rejected, and no test covers it. NovaCRM's KbChunk.embedding is a dimensionless vector whose rows carry a dims column, searched with cosine distance. Done means dimensionless vector columns are officially supported end to end, the docs match the code, and the index limits are stated.

## Acceptance criteria

- [ ] docs/TYPE_MAPPING.md, docs/ai-guide.md and docs/capabilities.md describe dimensionless vector columns as supported, with their limits (no HNSW or IVFFlat index on the bare column)
- [ ] generate emits a plain vector column for a vector field without dim (golden test)
- [ ] migrate diff and apply create and round-trip a dimensionless vector column with no spurious change on the next diff (integration test on CI)
- [ ] a CosineDistance search over rows of mixed dimensions filtered by a dims column returns the right rows (integration test on CI)
- [ ] an HNSW or IVFFlat index on a dimensionless column is refused with a clear error naming the column (test)

## Evidence
