# momo.yaml as library options and typed migrate State constants (#101)

Status: done 2026-11-11
Created: 2026-11-11
Epic: -
Sprint: 009-fix-every-open-github-issue-86-to-108-the-kuvryn-orbit-port
Type: bug
Severity: s2

## Description

GitHub issue #101. An app that migrates in-process at startup (`migrate.NewEngine(driver, embeddedDir, ...)`) has to repeat `migrate.exclude`, `introspect.only`, `external_extensions` and the timeouts of `momo.yaml` in Go, and keep a test that parses the file to hold the copy equal. Reproduce: try to build the engine options of an embedded `momo.yaml` from outside package `cli`; there is no exported function. Also `VersionStatus.State` is a `State`, but the only constants are the untyped history-row `Status*` ones, so there is nothing typed to compare it against.

Observed: the YAML-to-`migrate.Config` mapping is the unexported `cli.engineOptions`; no `migrate.State*` constants exist. Expected: an exported function that returns the options from a `fs.FS` file, the same one the CLI uses, and typed `StatePending`, `StateApplied`, `StateDirty`, `StateMismatch`, `StateMissing`.

Root cause: the mapping was written inside the CLI. Library code cannot import `cli`, and `gen/config` (the YAML loader) pulls in the whole generator, so `migrate` should not import it either. The mapping now lives in `migrate` (`Settings.Options`), `ConfigFromFile` decodes the migrate-relevant slice of the file, and the CLI feeds its already validated `momo.yaml` through the same `Settings.Options`.

## Acceptance criteria

- [x] a regression test pins the fix: red before the change, green after
- [x] `migrate.ConfigFromFile(fsys, path)` returns the engine options of a momo.yaml; the CLI and the library produce equal session and inspect options from the same file
- [x] a missing version, an empty file, a bad duration and an empty `introspect.only` glob are errors
- [x] `migrate.StatePending/Applied/Dirty/Mismatch/Missing` are typed `State` constants equal to the strings `Status` returns, and `Status` uses them
- [x] docs/upgrading.md (0.8.0 section) and docs/migrations.md describe both; docs/reference.md regenerated from CI

## Evidence

- Red: ci-agent run 42978182800 on worktree-agent-red-101 (test-only commit ba8a74b); unit job failed to compile: `cli/migrate_yamlconfig_test.go:66:26: undefined: migrate.ConfigFromFile`, `:111:11: undefined: migrate.StatePending` (and the other State constants). Branch deleted.
- Green: ci-agent run 43004067209, success, headSha dc85ac0a696164639e203f873759c05414689d81 (TestConfigFromFileEqualsCLIOptions, TestConfigFromFileRejectsBadFiles, TestStateConstants pass in unit and integration jobs).
- Earlier failures on the way: nested go.sum for go.yaml.in/yaml/v3 in the gorm, bun and ent fixtures (taken from the CI `go mod tidy -diff` output), and docs checks (qualified names, regenerated docs/reference.md from the regenerated-docs artifact of run 42997739150, whose sha b37e523 had the same Go code).
- Docs changed: docs/upgrading.md (Upgrading to 0.8.0), docs/migrations.md, docs/reference.md.
