# generate --check reports CRLF checkouts as stale on Windows

Status: open
Created: 2026-11-05
Epic: momo-02
Sprint: -
Type: bug
Severity: s3

## Description

Reproduction: in a consuming repository without a `.gitattributes` rule, check out on Windows with Git for Windows' default `core.autocrlf=true` (or clone anywhere with `git -c core.autocrlf=true clone`). Run `momo generate --check`.

Observed: every committed `momo_gen.go` is reported stale, because the files on disk have CRLF line endings while the generator emits LF, and the comparison is byte-for-byte. `momo generate` without `--check` then rewrites them with LF, which git shows as a whole-file change. Momo's own CI hit this (PR #4, windows job) and was fixed for this repository only, with `.gitattributes` `* text=auto eol=lf`.

Expected: `--check` treats a file that differs from the generated output only in CRLF versus LF line endings as up to date. A plain `momo generate` leaves such a file alone. The getting-started docs recommend a `.gitattributes` rule for generated files.

## Acceptance criteria

- [ ] a regression test pins the fix: red before the change, green after
- [ ] `momo generate --check` on a tree whose `momo_gen.go` files are the current output with CRLF endings exits 0 and lists nothing stale
- [ ] `momo generate` does not rewrite a file whose only difference is CRLF versus LF
- [ ] a file that differs in content (beyond line endings) is still reported stale
- [ ] the generator documentation recommends `*_gen.go text eol=lf` (or equivalent) in `.gitattributes`

## Evidence
