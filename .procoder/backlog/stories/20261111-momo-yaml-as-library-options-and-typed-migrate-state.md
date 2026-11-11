# momo.yaml as library options and typed migrate State constants (#101)

Status: open
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

- [ ] a regression test pins the fix: red before the change, green after
- [ ] `migrate.ConfigFromFile(fsys, path)` returns the engine options of a momo.yaml; the CLI and the library produce equal session and inspect options from the same file
- [ ] a missing version, an empty file, a bad duration and an empty `introspect.only` glob are errors
- [ ] `migrate.StatePending/Applied/Dirty/Mismatch/Missing` are typed `State` constants equal to the strings `Status` returns, and `Status` uses them
- [ ] docs/upgrading.md (0.8.0 section) and docs/migrations.md describe both; docs/reference.md regenerated from CI

## Evidence

<!-- Filled at close time. -->
