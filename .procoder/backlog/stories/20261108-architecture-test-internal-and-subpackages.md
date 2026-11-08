# Architecture test: internal never imports root, subpackages use exported API only

Status: done
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: 006-sprint-6-split-the-root-package-into-subpackages-and-keep

## Description

Extend `TestImportGraph` in `internal/repocheck/importgraph_test.go` (it currently pins what the core may import) with the two rules the split depends on. First, no package under `internal/` that the root imports may itself import `github.com/azrtydxb/momo`, so the dependency arrow stays one-way. Second, every feature subpackage listed in `.procoder/research/package-split.md` (`arrayop`, `fts`, `jsonop`, `agg`, `timeop`, `sqlfn`, `advisory`, `vecsearch`, `vecop`, `obs`) may import `momo` and the existing leaf packages (`dialect`, `types`, `vector`, `schema`) but nothing else in the module, and none of them may reference an unexported root name. The second rule is automatic in Go for a separate package, so the real check is the first, plus a list test: a directory under the module root that is not in the known list of core, leaf and feature packages fails, which forces a deliberate decision for every new package. The test is written before any package is moved, with the feature list empty, so each move story only adds its name.

## Acceptance criteria

- [x] `TestInternalNeverImportsRoot` runs `go list -deps` for every package under `internal/` that is not a test fixture module, and fails on any that depends on the root import path (red evidence on CI with a throwaway import added to `internal/golden`)
- [x] `TestFeaturePackagesImportOnlyAllowed` takes the feature package list from `internal/repocheck/testdata/packages.txt` and fails if one imports a module package outside `momo`, `dialect`, `types`, `vector`, `schema` and `internal/*`
- [x] `TestKnownPackages` fails when a new directory with Go files appears that is not listed in `packages.txt`, naming the file to edit
- [x] the existing `TestImportGraph` core rule is kept unchanged and still passes
- [x] the test uses `go list` through `os/exec` like the current one and does not need the integration database

## Evidence

- Red: ci-agent run 42494353060 on throwaway branch worktree-agent-a5cbf86e0b86d9a97-red (headSha abe7617), with a blank import of the root planted in internal/golden: `TestInternalNeverImportsRoot` failed in unit, postgres, mysql97 and mariadb ("internal/golden depends on the root"). Branch deleted.
- Green: ci-agent run 42494363907, headSha 70f5f97, all jobs success. Covers all five criteria: `TestInternalNeverImportsRoot`, `TestFeaturePackagesImportOnlyAllowed` (reads role `feature` from internal/repocheck/testdata/packages.txt, currently empty; `TestArchitectureRulesCatchViolations` plants a violation per rule), `TestKnownPackages` (names packages.txt in the error), `TestImportGraph` untouched, go list through os/exec, no database.
