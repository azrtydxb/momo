# naming ToSnake splits plural initialisms

Status: done 2026-11-11
Created: 2026-11-11
Epic: -
Sprint: 009-fix-every-open-github-issue-86-to-108-the-kuvryn-orbit-port
Type: bug
Severity: s2

## Description

GitHub issue #107, reported by the Kuvryn Orbit port (v0.7.0).

Reproduction: `naming.ToSnake("RestrictRoleIDs")`, or a model or plain struct
with a field `RestrictRoleIDs` under the default naming preset.

Observed vs expected: the column is `restrict_role_i_ds`; it should be
`restrict_role_ids`. A lone lower-case plural `s` after an initialism (IDs,
URLs, UUIDs, APIs) belongs to it.

Root cause: `ToSnake` in gen/naming/momo.go starts a word at an upper-case
letter that is followed by a lower-case one, so the `D` of `IDs` starts the
word `ds`. It has no word list. `raw_struct.go` carried its own copy of the
same rule (pinned to `ToSnake` by a test), and the `--from-db` field naming
(gen/fromdb/names.go) had its own, shorter list.

Fix: `ToSnake` finds an upper-case run that ends in a word of a shared list
(`internal/snake`) followed by a lone `s`, keeps the `s` with it and
breaks before the initialism. `--from-db` takes its field words from the same
list, so `role_ids` gives `RoleIDs` and back. The root package now calls
`internal/snake` instead of its copy (the core may not import gen/naming).

## Acceptance criteria

- [x] a regression test pins the fix: red before the change, green after
- [x] a word-list test covers RoleIDs, URLs, UUIDs, APIKeys, HTTPServer, ID,
      UserID and round-trips them through the --from-db naming
- [x] every generator golden stays the same, or each change is named in the
      evidence
- [x] docs/upgrading.md (0.8.0) lists the column names that change and says how
      to keep an old name with a `column:` tag

## Evidence

- Red: test-only commit 40e85f6 on throwaway branch worktree-agent-red-107, ci-agent run 42978193099 failed: TestToSnakeInitialisms (ToSnake("RoleIDs") = "role_i_ds", "APIs" = "ap_is", "HTTPIDs" = "httpi_ds", "RestrictRoleIDs" = "restrict_role_i_ds") and TestFieldNameRoundTripsThroughToSnake (pascal("role_ids") = "RoleIds", pascal("urls") = "Urls"). Branch deleted.
- Green: run 42999792466 on c4a7d80 (branch worktree-agent-a565a4b39df5f7711), all ci-agent jobs green. Earlier runs on this branch failed for fixable reasons only: the core may not import gen/naming (moved to internal/snake), docs/reference.md rows, and the googleuuid golden.
- Goldens: the only changed golden is gen/fromdb/googleuuid_test.go (TeamIds became TeamIDs in TestGenerateFromDBGoogleUUID and TestGenerateFromDBDefaultUUIDIsMomo, the --from-db field name for team_ids); every other generator golden is unchanged. The column-name change and the --from-db field-name change are separate sections in the upgrade notes.
- Docs: docs/upgrading.md (0.8.0: the column names that change, the --from-db field renames, how to keep an old name with a column: tag or introspect.columns match/name), docs/ai-guide.md.
