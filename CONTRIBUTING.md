# Contributing to Momo

## Prerequisites

- Go 1.26 or later
- A C toolchain (for the SQLite packages, which use cgo)
- Docker (for `make db-up`, the local integration databases)

## Commands

- `make test`: unit tests, including the separate module `internal/integration` that holds the tests needing gqlgen, gRPC, protoc or the OpenAPI validator
- `make test-race`: unit tests with the race detector
- `make test-compilefail`: compile-fail tests for the typed API
- `make test-integration`: integration tests against real databases

Every test that needs a database lives in a file that starts with the `//go:build integration` tag, next to the SPDX line, and gets its database from `internal/testdb` or `testopen`. `make test`, and the macOS and Windows CI jobs, build without the tag and so run unit tests only; they cannot skip a database test silently because they never compile one. `make test-integration` sets `MOMO_REQUIRE_INTEGRATION=1`, so an unset database URL fails the run. `TestIntegrationTagConvention` in `internal/repocheck` fails when a file that uses these helpers lacks the tag.

Fuzz targets are listed in `FUZZ_TARGETS` in the Makefile and `make test-fuzz` runs each one for `FUZZTIME`, an execution count. `TestFuzzTargetsListed` fails when a `Fuzz` function is missing from the list.

Dialect coverage is expressed as subtests from `testopen.Each`, one per dialect built into this version; integration tests have no name suffix. `make test-integration-<dialect>` sets that dialect's URL variables and runs the whole suite.

## Documentation pages and Go code blocks

A new page under `docs/` must be added to the `nav` in `mkdocs.yml` and linked from `docs/README.md`; `TestEveryDocPageInNav` and `TestDocsIndexLinksEveryPage` fail otherwise. Every Go block in `docs/` is checked by `TestDocsCodeBlocks`: write a complete `package main` program (it is compiled), or, for a snippet that cannot stand alone, put the comment `<!-- docscheck:fragment -->` on the line before the fence (it must still parse as Go). The number of fragments is capped by `maxFragmentBlocks` in `internal/docscheck/codeblocks_test.go`, so prefer a program. A block marked `<!-- docscheck:run -->` is also executed against PostgreSQL.

## Exported API changes need a docs change

The `apidiff-docs` CI job (`make apidiff-gate`, `scripts/apidiff-docs-gate.sh`) runs `golang.org/x/exp/cmd/apidiff`, pinned in the Makefile, between the base branch and your branch for the root package and every non-internal subpackage. Any exported change, compatible or incompatible, fails the job unless one of two things holds:

- the diff touches `README.md`, `llms.txt`, a Markdown page under `docs/` (`docs/**/*.md`) or an `examples/**/README.md` (other files under `docs/` and `examples/` do not count); or
- the pull request body, or a commit message in the range, has a line that starts in column 0 with `Docs: no reader-visible change: ` followed by a real reason, for example `Docs: no reader-visible change: internal helper exposed for tests`. A blank reason, a `<placeholder>` such as `<reason>`, and an indented or quoted line are rejected.

A change to a Go comment or an unexported identifier produces an empty report and needs neither. The job lives in its own workflow, `.github/workflows/apidiff-docs.yml`, so it also runs when the PR body is edited: add the escape line to the body and the check re-evaluates without a new commit. The job reads the live body, not the stale event payload. It keeps one comment (marked `<!-- apidiff-docs -->`) with the apidiff report, and posts none when the API did not change. Run it locally with `make apidiff-gate`.

## Root package file names

Every non-test `.go` file in the repository root is `doc.go` or is named with one of the prefixes `db`, `tx`, `driver`, `err`, `build`, `meta`, `expr`, `col`, `select`, `insert`, `update`, `delete`, `upsert`, `write`, `raw` or `rel`, either bare (`db.go`) or followed by an underscore and a topic (`expr_ops.go`). A test file is named after its subject file: `foo_test.go`, `foo_internal_test.go` or `foo_integration_test.go` needs `foo.go`. `TestRootFilePrefixes` in `internal/repocheck` fails on any other name and lists the prefixes in its message.

Files that predate the rule are listed in `internal/repocheck/testdata/prefix_exceptions.txt`. The list may only shrink: a listed file that no longer exists fails the test, and a name that is not in the frozen snapshot `prefix_frozen.txt` is rejected, so the list cannot admit a new file. Rename the file instead of adding it to the list.

## Commit messages and PR titles

Commit messages and PR titles use Conventional Commits: `feat`, `fix`, `perf`,
`refactor`, `docs`, `test`, `build`, `ci`, `chore`, `revert`. Mark breaking
changes with `feat!:` or a `BREAKING CHANGE:` footer.

## SPDX headers

Every new `.go` file starts with this header, followed by a blank line before
`package`:

```go
// SPDX-License-Identifier: Apache-2.0

package example
```

Generated files begin with `// Code generated by momo` instead.
