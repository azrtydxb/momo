# fromdb singularizer names table releases as Releas

Status: open
Created: 2026-11-11
Epic: -
Sprint: 008-sprint-8-fix-the-kuvryn-orbit-port-issues-73-to-79-and-the
Type: bug
Severity: s3

## Description

GitHub issue #82, reported by the Kuvryn Orbit port.

Reproduction: run `momo generate --from-db` against a database with a table
named `releases`, using the default naming preset.

Observed vs expected: the generated model is named `Releas`; it should be
`Release`. `singulars` in gen/fromdb/names.go offers the `-es` strip before
the `-s` strip, and the caller keeps the first candidate whose plural under the
preset gives the table name back. The momo preset pluralizes `releas` to
`releases`, so the wrong candidate wins. Every word ending in `-se` is
affected: releases, cases, courses, responses, licenses, bases, houses,
purposes. Words that really take `-es` must keep working: addresses, classes,
statuses, buses, boxes, matches, wishes, quizzes. Orbit works around it with an
explicit `name: Release` for the table in momo.yaml.

## Acceptance criteria

- [ ] a regression test pins the fix: red before the change, green after
- [ ] a word-list test covers both groups above under every naming preset
      --from-db supports, and each word gives the expected model name
- [ ] the existing from-db goldens are unchanged, or each change is a
      corrected name named in the evidence
- [ ] docs/upgrading.md (0.7.0) notes that generated model names for `-se`
      words change, and how to keep the old name

## Evidence
