# Repocheck: file-prefix guard for the root package

Status: open
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: -

## Description

The root package has 57 non-test files with no naming rule, so a reader cannot tell which file holds what. Add a repocheck test that enforces the taxonomy in section 5 of `.procoder/research/package-split.md`: every non-test root file is `doc.go` or is named with one of the allowed prefixes (`db`, `tx`, `driver`, `err`, `build`, `meta`, `expr`, `col`, `select`, `insert`, `update`, `delete`, `upsert`, `write`, `raw`, `rel`), either bare or followed by an underscore and a topic. Test files take the subject file name plus `_test`, `_internal_test` or `_integration_test`. The guard starts in ratchet mode: today's non-conforming files are listed in `internal/repocheck/testdata/prefix_exceptions.txt`, the list may only shrink, and a new non-conforming file fails the test. Story root-file-prefix-rename empties the list and removes the ratchet. Done means a developer adding `foo_bar.go` to the root gets a failing test with the allowed prefixes in the message.

## Acceptance criteria

- [ ] `TestRootFilePrefixes` in `internal/repocheck` fails when a new root file named `zzz_probe.go` is added (red evidence: a throwaway branch run on CI), and the failure message lists the allowed prefixes
- [ ] a file named in `prefix_exceptions.txt` that no longer exists fails the test (the list cannot grow stale)
- [ ] adding a file to `prefix_exceptions.txt` is rejected unless the file already existed on main (the test reads the list and the directory, so the list cannot be used to admit new files)
- [ ] test files must match their subject: `foo_integration_test.go` requires `foo.go` or a documented exception, checked by the same test
- [ ] the test runs only over the root package directory, ignores `testdata`, and passes on current main with the exception list filled in
- [ ] `CONTRIBUTING.md` has a short section naming the prefixes and the exception-list rule

## Evidence
