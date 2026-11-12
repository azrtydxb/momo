# fromdb names singular -ss tables Clas

Status: done 2026-11-11
Created: 2026-11-11
Epic: -
Sprint: 009-fix-every-open-github-issue-86-to-108-the-kuvryn-orbit-port
Type: bug
Severity: s2

## Description

GitHub issue #100, reported by Kuvryn-Owl (0.7.0), the same family as #82.

Reproduction: `momo generate --from-db` with `naming: gorm` (or momo) against a
table named `class` or `assessment_class`.

Observed vs expected: the model is `Clas` (package `clas`) and
`AssessmentClas`; it should be `Class` and `AssessmentClass`. Any singular
table in `-ss` (class, address, access, process, glass, boss) is affected, and
`status` gives `Statu`.

Root cause: `singulars` in gen/fromdb/names.go strips the `s` of every word
ending in `s`, and `structName` keeps the first candidate when none gives the
table name back under the preset. A singular `-ss` word never round-trips (its
plural is `-sses`), so the wrong first candidate `clas` wins.

Fix: a word in `-ss`, and the listed singular `-us` words (status, bus, campus,
virus, ...), are offered unchanged and only unchanged. Plurals (classes,
statuses, menus) go through the old rules. The 13 words that worked before are
already in the word-list test and stay there.

## Acceptance criteria

- [x] a regression test pins the fix: red before the change, green after
- [x] the word-list test keeps releases, cases, addresses, classes, statuses,
      buses, boxes, matches, quizzes, aliases, biases, caches and houses, and
      adds the singular class, address, status, glass, boss, process and
      access, each under every naming preset
- [x] the existing from-db goldens are unchanged, or each change is a
      corrected name named in the evidence
- [x] docs/upgrading.md (0.8.0) notes that generated model names for singular
      `-ss` tables change, and how to keep the old name

## Evidence

- Red: test-only commit on throwaway branch worktree-agent-red-100, ci-agent run 42978257945 failed TestStructNameSingularWords: class gave Clas, address Addres, status Statu, glass Glas, boss Bos, user_status UserStatu (momo, gorm, bun, prisma). Branch deleted.
- Green: run 42999792466 on c4a7d80 (branch worktree-agent-a565a4b39df5f7711), all ci-agent jobs green. Earlier runs on this branch failed for fixable reasons only: the core may not import gen/naming (moved to internal/snake), docs/reference.md rows, and the googleuuid golden.
- Goldens: no from-db golden changed for this fix; the 13 words that worked before were already in the word-list test and still pass.
- Docs: docs/upgrading.md (0.8.0, singular -ss and -us table names).

- Fix round (review): the -us rule is inverted (kept unless a plural of a -u word: menus, emus, gnus, haikus, tutus, kudus) and explicit keep-lists cover -is, -as and -ns words; areas and ideas still lose their s.
- Fix round red: test-only commit 1fca50cb on throwaway branch worktree-agent-red-naming2, ci-agent run 43005590287 failed TestStructNameSingularWords (nucleus = "Nucleu", stimulus = "Stimulu", analysis = "Analysi", axis = "Axi", alias = "Alia", gas = "Ga", news = "New", lens = "Len"). Branch deleted.
- Fix round green: ci-agent run 43012005078 on b5196ce (merge of origin/sprint-9 at 009f86a7, all jobs green, gen-check clean so docs/reference.md needed no foreign rows).
