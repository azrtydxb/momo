# fromdb singularizer names table releases as Releas

Status: done 2026-11-11
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

- [x] a regression test pins the fix: red before the change, green after
- [x] a word-list test covers both groups above under every naming preset
      --from-db supports, and each word gives the expected model name
- [x] the existing from-db goldens are unchanged, or each change is a
      corrected name named in the evidence
- [x] docs/upgrading.md (0.7.0) notes that generated model names for `-se`
      words change, and how to keep the old name

## Evidence

- Red: test-only commit 20e56de (word list, gen/fromdb/names_singular_test.go) on a
  throwaway branch, ci-agent run 42930526482, job unit failed with
  TestStructNameSingularWords: releases gave Releas (also cases Cas, courses
  Cours, responses Respons, licenses Licens, bases Bas, houses Hous, purposes
  Purpos, uses Us) under momo and prisma, and quizzes gave Quizze (gorm, bun)
  or Quizz (prisma). Branch deleted.
- Green: fix commit e314398 on worktree-agent-ae07bd95b0b4ad34b, ci-agent run
  42930572993, all 9 jobs green. singulars now prefers the -s strip and offers
  the -es strip first only for -sses/-xes/-zes/-ches/-shes and -uses (not
  -ouses/-auses/uses); "quizzes" is an irregular.
- Presets that cannot round-trip a word are skipped with a reason in the test:
  ent for statuses, buses and quizzes (case-sensitive rules); momo for quizzes
  (quiz becomes quizes).
- Goldens: unit (including the from-db goldens) passed with no golden file
  changed; no name was corrected in a golden.
- Known limit: -uses words that are really -use plurals (excuses, fuses) still
  take the -es strip, and -ases words such as aliases give Aliase.
- docs/upgrading.md 0.7.0 has the new subsection, quoting
  `introspect: tables: <name>: { name: ... }` from docs/migrations.md.
