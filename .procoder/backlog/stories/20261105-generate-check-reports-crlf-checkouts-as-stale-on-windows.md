# generate --check reports CRLF checkouts as stale on Windows

Status: open
Created: 2026-11-05
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites
Type: bug
Severity: s3

## Description

Reproduction: in a consuming repository without a `.gitattributes` rule, check out on Windows with Git for Windows' default `core.autocrlf=true` (or clone anywhere with `git -c core.autocrlf=true clone`). Run `momo generate --check`.

Observed: every committed `momo_gen.go` is reported stale, because the files on disk have CRLF line endings while the generator emits LF, and the comparison is byte-for-byte. `momo generate` without `--check` then rewrites them with LF, which git shows as a whole-file change. Momo's own CI hit this (PR #4, windows job) and was fixed for this repository only, with `.gitattributes` `* text=auto eol=lf`.

Expected: `--check` treats a file that differs from the generated output only in CRLF versus LF line endings as up to date. A plain `momo generate` leaves such a file alone. The getting-started docs recommend a `.gitattributes` rule for generated files.

## Acceptance criteria

- [x] a regression test pins the fix: red before the change, green after
- [x] `momo generate --check` on a tree whose `momo_gen.go` files are the current output with CRLF endings exits 0 and lists nothing stale
- [x] `momo generate` does not rewrite a file whose only difference is CRLF versus LF
- [x] a file that differs in content (beyond line endings) is still reported stale
- [x] the generator documentation recommends `*_gen.go text eol=lf` (or equivalent) in `.gitattributes`

## Evidence

Fix: `Writer.Commit` in gen/writer.go normalizes the existing file's CRLF to LF before comparing with the (LF) generated output. `hasHeader` and orphan detection already worked on CRLF files (the header prefix is matched on the first line, which only gains a trailing CR) and are now pinned by tests.

- Regression tests: gen/writer_crlf_test.go (TestWriterTreatsCRLFAsUpToDate, TestWriterCRLFContentDriftStillStale) and cmd/momo/generate_crlf_test.go (TestGenerateCheckCRLFCheckout). Red before the change (`CRLF file reported changed`), green after. Mutation check: reverting the normalization makes both packages' CRLF tests fail.
- CRLF tree: TestGenerateCheckCRLFCheckout converts every generated momo_gen.go to CRLF; `--check` exits 0 with nothing stale.
- No rewrite: the same test runs plain `generate` and asserts the bytes are unchanged; the writer test also asserts mtime is unchanged.
- Drift: a CRLF file with an extra line is still reported `stale: <path>` (exit 1), and rewriting emits LF.
- Docs: no generator page exists under docs/ yet, so the `momo generate` help text (cmd/momo/generate.go) recommends `*_gen.go text eol=lf` in `.gitattributes`. Follow-up: the getting-started README, being written by another agent, should repeat this recommendation.
