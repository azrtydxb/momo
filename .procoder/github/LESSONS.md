# Lessons — findings that escaped our own gates

One entry per finding caught downstream (bot review, human review,
production) — the escape is the bug; the finding is its symptom. Every
entry names which layer should have caught it and the adaptation that now
does. `procoder lessons` flags entries with no adaptation.

Entry shape (unindented in real entries):

    ## <date> <where caught> — <one-line finding>

    - Class: mechanical | judgment | taste
    - Missed by: linter | rubric | controller | test | ci
    - Adaptation: <the concrete change that catches this class from now on>

## 2026-11-05 PR #1 github-advanced-security (gosec) — 44 G104 discarded-error annotations

- Class: mechanical
- Missed by: linter
- Adaptation: `make gosec` (pinned gosec v2.30.0, same release as the CI
  workflow) fails on any finding, `TestMakefileTargets` pins the target,
  and REVIEW.md requires it clean before review. gosec v2.29.0 could not
  type-check under Go 1.27 and silently passed, so both pins moved to
  v2.30.0, verified by a probe file that must fail with G104.
