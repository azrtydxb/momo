# CLI: let users run custom generator plugins (exported cli.Main(plugins...))

Status: open
Created: 2026-11-05
Epic: momo-02-generator
Sprint: -

## Description

The `momo` binary runs only its built-in plugins (`plugins()` in `cmd/momo/cmd.go`), and naming any other plugin in `momo.yaml` is an error that points users to the Go API (`gen.Emit` with plugins). That is enough for v0.1, but a team that writes its own `gen.Plugin` has to hand-write a driver that re-implements config discovery, pattern handling, `--check`, exit codes and diagnostics formatting. Done means a user can build their own binary with `cli.Main(plugins...)` and get exactly the `momo generate` behavior, with their plugins selectable from `momo.yaml`, without forking the CLI.

## Acceptance criteria

- [ ] An exported entry point (for example package `github.com/azrtydxb/momo/cli`, `func Main(plugins ...gen.Plugin)` plus a testable `Run(args, stdout, stderr, plugins...) int`) exists, and `cmd/momo` becomes a thin caller of it with the built-in plugins
- [ ] A test builds a custom binary from a fixture module containing a fixture plugin, runs `generate` with a `momo.yaml` that enables the plugin, and asserts the plugin's output file exists and `generate --check` exits 0
- [ ] The same test asserts that a plugin not passed to `Main` still fails with exit 2 and the Go API hint
- [ ] `momo help` and the package godoc describe how to build a custom binary
- [ ] `procoder check` gate is clean and the commit is made

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->
