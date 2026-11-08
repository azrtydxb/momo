# docscheck sees list-indented go fences

Status: open
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: -

## Description

A docs reader copies Go examples from pages such as `docs/migrations.md` and
`docs/testing.md` and expects them to compile. The code-block check in
`internal/docscheck/codeblocks_test.go` matches fences with the pattern `^```go\s*$`, so a
fence indented inside a list item is invisible to it and never compiled or
counted against `maxFragmentBlocks`. Two such blocks exist today
(`docs/migrations.md` near line 875 and `docs/testing.md` line 26). Found by the
second re-review of the SQLite migrations branch (stories 192-195).

Done means every Go code fence in the docs, indented or not, is either a complete
program that the check compiles or carries `<!-- docscheck:fragment -->`.

## Acceptance criteria

- [ ] The fence pattern in `internal/docscheck` accepts leading whitespace and
      strips the list indent from the block body before compiling it; a unit
      test with an indented fence fails on the old pattern (red run on CI) and
      passes on the new one.
- [ ] The two indented blocks in `docs/migrations.md` and `docs/testing.md`
      compile or are marked as fragments, and `maxFragmentBlocks` matches the
      new count.
- [ ] The docs code-block test is green on CI on the merged branch.

## Evidence
