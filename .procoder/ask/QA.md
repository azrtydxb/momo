# Questions procoder cannot answer for you

Written 2026-11-06 10:05 UTC.

Answer each one by writing a line beginning `Answer: ` under it, then
hand the file back with `procoder ask --file .procoder/ask/QA.md`.
Leave the `Key:` lines alone — they are what ties an answer to its question.

## Q1: [decision] decisions.md

Key: 13214d8ab382
Question: Where story verification runs

- Push each story branch (and sprint-2) to origin so GitHub Actions runs the full suite, race, lint and the pg16/pg17 matrix; agents run only targeted tests locally (recommended)
- Keep everything local, but cap concurrent agents at two and move GOCACHE off the full DATA disk
- Keep the current setup

Decision (2026-11-06): push branches, use CI; standing approval to push worktree-agent-* story branches and sprint-2 to origin (no PRs or merges to main without asking); local runs limited to the story's own tests.

Answer: Push branches, use CI (user, AskUserQuestion 2026-11-06): standing approval to push worktree-agent-* story branches and sprint-2; no PRs or merges to main without asking; local runs limited to the story's own tests.

## Q2: [decision] decisions.md

Key: d2145a7f634b
Question: v0.1.0 scope and release push

- Lock scope: merged relations plus stories 149, 154 and 150; stories 151, 152 and 155 move to 0.2; approve pushing sprint-2 and opening the PR into main once the scope is done (recommended)
- Lock that scope, but ask again before pushing
- Wait for all of momo-06 (stories 150 to 155) before release

Decision (2026-11-06): wait for all relations (stories 150 to 155) before v0.1.0.

Answer: Wait for all relations (user, AskUserQuestion 2026-11-06): stories 150 to 155 before v0.1.0.
