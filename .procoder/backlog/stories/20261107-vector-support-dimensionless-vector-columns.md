# Vector: support dimensionless vector columns

Status: done 2026-11-07
Created: 2026-11-07
Epic: momo-07-vectors-pgvector
Sprint: 005-sprint-5-security-and-maintainability-hardening

## Description

GitHub issue #29, found by the NovaCRM port. A pgvector column declared without `dim` is accepted by the code (gen/schema.go emits plain `vector`, vector_column.go treats a zero Dim as unchecked) while docs/TYPE_MAPPING.md and docs/ai-guide.md say it is rejected, and no test covers it. NovaCRM's KbChunk.embedding is a dimensionless vector whose rows carry a dims column, searched with cosine distance. Done means dimensionless vector columns are officially supported end to end, the docs match the code, and the index limits are stated.

## Acceptance criteria

- [x] docs/TYPE_MAPPING.md, docs/ai-guide.md and docs/capabilities.md describe dimensionless vector columns as supported, with their limits (no HNSW or IVFFlat index on the bare column)
- [x] generate emits a plain vector column for a vector field without dim (golden test)
- [x] migrate diff and apply create and round-trip a dimensionless vector column with no spurious change on the next diff (integration test on CI)
- [x] a CosineDistance search over rows of mixed dimensions filtered by a dims column returns the right rows (integration test on CI)
- [x] an HNSW or IVFFlat index on a dimensionless column is refused with a clear error naming the column (test)

## Evidence

CI run 42410769232 (workflow ci-agent) on 663a2db (review-fix round, merged with sprint-5): success. Earlier run 42409354133 on 7ba44fe: success.

- Docs: docs/TYPE_MAPPING.md, docs/ai-guide.md, docs/capabilities.md and docs/vectors.md (new Dimensionless columns section).
- Golden: gen TestEmitDescriptorsDimensionlessVector and TestDimensionlessVectorColumns.
- Round trip: migrate/postgres TestDimensionlessVectorRoundTrip (integration).
- Search: TestCosineDistanceDimensionlessColumn (integration), plus the unit test TestVectorDistanceDimensionless.
- Refusal: schema TestVectorIndexRefusesDimensionlessColumn and the gen index diagnostics.

## Out of scope

The optional follow-up in the issue, allowing an expression or partial index such as an HNSW index on the embedding cast to a fixed size with a dims predicate, is not part of this story. Expression parts stay rejected by the vector index validation.

## Notes

Support needed one code change: a distance on a column with dimension zero validated its argument against zero and so rejected every vector. A zero dimension now means the argument is checked against its own size (NaN, Inf and emptiness are still refused).
