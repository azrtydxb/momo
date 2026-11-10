# momo-02 Task 25: Plan gate for the generator

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-02-generator Task 25: Plan gate for the generator (`.procoder/plans/momo-02-generator.md:2731`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-2, S-3, S-43, S-62, S-67, S-69, S-70, S-71. Contributes to S-2, S-3, S-43, S-62, S-67, S-69, S-70, S-71; those criteria are verified in the stories that define their named tests. Runs last in momo-02, after Tasks 26, 27, 29 and 30 (which wait for momo-05).

## Acceptance criteria

- [ ] Step 1: Run `go test -race -count=1 ./gen/... ./internal/golden ./internal/testmodels/... ./cmd/momo ./momo`; expect every package `ok`.
- [ ] Step 2: Run `go test -count=1 -v ./gen/... ./cmd/momo ./momo ./internal/testmodels/example -run 'TestEmitDescriptorsGolden|TestGenerateDiagnostics|TestGenerateDeterministic|TestGenerateCheck|TestGenerateCrossPackage|TestGenerateRefusesHandwritten|TestGenerateNameCollisions|TestEmbeddedAndMixins|Tes…
- [ ] Step 3: Run `go list -deps ./momo ./dialect ./schema ./vector ./types | grep -E 'golang.org/x/tools|yaml'`; expect no output (dependency rule 1).
- [ ] Step 4: Run `go vet ./...` and `gofmt -l .`; expect no output.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

Carry reason: the gate cannot pass as written. Step 2 names `TestViewModelsNotWritable`, `TestCompanionSchemaMethods` and `TestCompanionSchemaDiagnostics`, which do not exist (stories 050 and 051: Needs decision / carried). Steps 1, 3 and 4 use paths of the first plan (`./momo`, the root package was never moved there); the shipped equivalents run on every CI run: unit tests with `-race` (`make test-race`), the import-graph rules in `internal/repocheck` (dependency rule 1), and `go vet`/gofmt in the lint job (.github/workflows/ci.yml). Every other test named in Step 2 exists (checked with grep): `TestEmitDescriptorsGolden`, `TestGenerateDiagnostics`, `TestGenerateDeterministic`, `TestGenerateCheck`, `TestGenerateCrossPackage`, `TestGenerateRefusesHandwritten`, `TestGenerateNameCollisions`, `TestEmbeddedAndMixins`, `TestNamingPresets`, `TestColumnByName`, `TestFieldPermissions`, `TestGeneratorPlugin`, `TestTagVocabulary`, `TestParseTags`, `TestConfigLoad`, `TestNamingOverridesFromConfig`, `TestGoGenerateIntegration`, `TestCompanionDSLCompiles`, `TestRelationDescriptorShape`, `TestModelHookEmission`, `TestZeroHookSet`, `TestFieldPermissionsRuntime` (these last three added in this sprint, story 053 and 054), `TestGeneratedHeaderRegexp`, `TestCLIExitCodes`, `TestPluginsUseWriter`. Close this story after 050 and 051 are decided and the CI run of the sprint branch is green. Criteria left unticked.

