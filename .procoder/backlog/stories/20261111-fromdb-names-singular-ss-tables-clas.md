# fromdb names singular -ss tables Clas

Status: open
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

- [ ] a regression test pins the fix: red before the change, green after
- [ ] the word-list test keeps releases, cases, addresses, classes, statuses,
      buses, boxes, matches, quizzes, aliases, biases, caches and houses, and
      adds the singular class, address, status, glass, boss, process and
      access, each under every naming preset
- [ ] the existing from-db goldens are unchanged, or each change is a
      corrected name named in the evidence
- [ ] docs/upgrading.md (0.8.0) notes that generated model names for singular
      `-ss` tables change, and how to keep the old name

## Evidence

- Red:
- Green:
- Docs: docs/upgrading.md (0.8.0)
