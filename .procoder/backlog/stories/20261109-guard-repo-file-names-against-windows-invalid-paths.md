# Guard repo file names against Windows-invalid paths

Status: done
Created: 2026-11-09
Epic: momo-14-structure-and-docs
Sprint: 006-sprint-6-split-the-root-package-into-subpackages-and-keep

## Description

An agent committed a watcher log named `worktree-agent-adffe7ae4d2ad2c57:61fe0ce` at the repo root (commit 0ed67ea). Git on Linux holds a colon name without complaint, but the Windows job's checkout cannot create it and failed on sprint-6. The agent branch looked green because ci-agent has no Windows job. Add `TestRepoFileNamesAreWindowsSafe` to `internal/repocheck` (`filenames_test.go`): it walks every file and directory of the repo (skipping `.git`, `.claude`, `node_modules`, `vendor`) and fails, naming each offender, when a path component contains any of `: * ? " < > | \` or a control character, ends in a space or a dot, or is a Windows reserved name (CON, PRN, AUX, NUL, COM1-9, LPT1-9, case-insensitive, with or without extension). It also fails on two paths that differ only by case. The rules live in the pure function `fileNameProblems`, pinned by `TestFileNameRules`. The test runs in the unit job, so ci-agent catches the escape on Linux without a Windows runner.

## Acceptance criteria

- [x] a file `stray:name.txt` at the repo root fails the test with its path named (red evidence on CI from a throwaway branch)
- [x] the current tree passes, so the guard adds no exceptions list
- [x] `TestFileNameRules` pins every forbidden character class, the trailing space and dot rules, the reserved names with and without an extension (and `com10`, `lpt0` as valid), and the case-only collision
- [x] the test carries a "Catches:" comment naming the escape it prevents

## Evidence

- Red, ci-agent run 42598901541 on throwaway branch `worktree-agent-a2b6747832a88ad35-red` (headSha 896850f, deleted afterwards): the test plus a dummy `stray:name.txt`. The unit job failed with `--- FAIL: TestRepoFileNamesAreWindowsSafe` and `filenames_test.go:101: stray:name.txt: contains : (not creatable on a Windows checkout)`.
- Criterion 1: the red run above.
- Criterion 2: green ci-agent run 42598982565 (headSha 00164930383bd4fbc04e29c14918b9f655ac9144), the test alone on `worktree-agent-a2b6747832a88ad35`, all jobs success.
- Criterion 3: `TestFileNameRules` in `internal/repocheck/filenames_test.go`, passing in the same green run 42598982565.
- Criterion 4: the comment block above `windowsReserved` in `internal/repocheck/filenames_test.go`.
