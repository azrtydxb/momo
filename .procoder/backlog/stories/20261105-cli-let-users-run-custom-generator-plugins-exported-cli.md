# CLI: let users run custom generator plugins (exported cli.Main(plugins...))

Status: done
Created: 2026-11-05
Epic: momo-02-generator
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

The `momo` binary runs only its built-in plugins (`plugins()` in `cmd/momo/cmd.go`), and naming any other plugin in `momo.yaml` is an error that points users to the Go API (`gen.Emit` with plugins). That is enough for v0.1, but a team that writes its own `gen.Plugin` has to hand-write a driver that re-implements config discovery, pattern handling, `--check`, exit codes and diagnostics formatting. Done means a user can build their own binary with `cli.Main(plugins...)` and get exactly the `momo generate` behavior, with their plugins selectable from `momo.yaml`, without forking the CLI.

## Acceptance criteria

- [x] An exported entry point (for example package `github.com/azrtydxb/momo/cli`, `func Main(plugins ...gen.Plugin)` plus a testable `Run(args, stdout, stderr, plugins...) int`) exists, and `cmd/momo` becomes a thin caller of it with the built-in plugins
- [x] A test builds a custom binary from a fixture module containing a fixture plugin, runs `generate` with a `momo.yaml` that enables the plugin, and asserts the plugin's output file exists and `generate --check` exits 0
- [x] The same test asserts that a plugin not passed to `Main` still fails with exit 2 and the Go API hint
- [x] `momo help` and the package godoc describe how to build a custom binary
- [x] `procoder check` gate is clean and the commit is made

## Evidence

CI: ci-agent run 42773371002, headSha 5acef91 (all 9 jobs green, conclusion success). An earlier run (42769490088) failed only `TestPluginNotPassedToMain` (it built the fixture binary after chdir); fixed in 54e3d78.

- Entry point: package `cli` (cli/cmd.go) with `Main(plugins ...gen.Plugin)` and `Run(args, stdout, stderr, plugins ...gen.Plugin) int`; the whole former `cmd/momo` implementation moved there (tests and testdata with it), and cmd/momo/main.go is `cli.Main()`, a thin caller. The stock binary has no built-in plugins, so it passes none.
- `TestCustomBinary` (cli/main_test.go) builds cli/testdata/custombin (a `cli.Main(marker{})` binary with plugin `custom`), runs `generate` with a momo.yaml enabling it, asserts custom_gen.go exists, `generate --check` exits 0, and a drifted plugin file gives exit 1 with a `stale:` line.
- `TestPluginNotPassedToMain` asserts `Run` without the plugin, and the custom binary with an unknown plugin, exit 2 with `gen.Emit` (and `cli.Main`) in the message and nothing written.
- `momo help` text (cli/cmd.go usage, generate usage in cli/generate.go) and the package godoc (cli/cmd.go) describe building a binary with `cli.Main`. Docs updated: docs/capabilities.md, docs/ARCHITECTURE.md, docs/migrations.md; internal/gendocs reads dialectImpl from cli/; internal/repocheck/testdata/packages.txt lists `cli`.
- Commit gate: commits 2751e6e and 54e3d78 went through the procoder hooks.
