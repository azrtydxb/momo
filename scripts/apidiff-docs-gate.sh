#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
#
# Docs gate: an exported API change must come with a docs change.
#
# Compares the exported API of every non-internal package in the root module
# between the base and HEAD with golang.org/x/exp/cmd/apidiff. A non-empty
# report (compatible or incompatible) fails unless
#   1. the diff touches README.md, docs/**, llms.txt or examples/**, or
#   2. a commit message in the range, or the pull request body (PR_BODY), has a
#      line "Docs: no reader-visible change: <reason>".
#
# Environment:
#   APIDIFF   command that runs apidiff, e.g. "go run golang.org/x/exp/cmd/apidiff@<ver>" (required)
#   BASE_REF  ref to compare against; default: the origin/main or origin/sprint-* branch
#             with the fewest commits not in HEAD
#   PR_BODY   pull request body, searched for the escape line
#   REPORT    file to write the markdown report to (default apidiff-report.md)
set -euo pipefail

: "${APIDIFF:?APIDIFF must be set}"
REPORT=${REPORT:-apidiff-report.md}
PR_BODY=${PR_BODY:-}

if [ -z "${BASE_REF:-}" ]; then
	best=""
	bestn=999999
	for ref in $(git for-each-ref --format='%(refname:short)' refs/remotes/origin/main 'refs/remotes/origin/sprint-*'); do
		n=$(git rev-list --count "$ref..HEAD")
		if [ "$n" -lt "$bestn" ]; then
			best=$ref
			bestn=$n
		fi
	done
	BASE_REF=$best
fi
[ -n "$BASE_REF" ] || {
	echo "apidiff gate: no base ref found" >&2
	exit 2
}
BASE_SHA=$(git merge-base "$BASE_REF" HEAD)
MOD=$(awk '$1 == "module" { print $2; exit }' go.mod)
echo "apidiff gate: base $BASE_REF ($BASE_SHA), module $MOD"

tmp=$(mktemp -d)
cleanup() {
	git worktree remove --force "$tmp/base" >/dev/null 2>&1 || true
	rm -rf "$tmp"
}
trap cleanup EXIT

git worktree add --detach "$tmp/base" "$BASE_SHA" >/dev/null
(cd "$tmp/base" && $APIDIFF -m -w "$tmp/base.export" "$MOD")
$APIDIFF -m "$tmp/base.export" "$MOD" >"$tmp/report.txt"

if ! grep -q '[^[:space:]]' "$tmp/report.txt"; then
	echo "apidiff gate: no exported API change"
	printf '## apidiff\n\nNo exported API change against `%s`.\n' "$BASE_REF" >"$REPORT"
	exit 0
fi

docs_changed=$(git diff --name-only "$BASE_SHA" HEAD | grep -E '^(README\.md|docs/|llms\.txt|examples/)' || true)
escape=$({
	printf '%s\n' "$PR_BODY"
	git log --format=%B "$BASE_SHA..HEAD"
} | tr -d '\r' |
	grep -E '^[[:space:]]*Docs: no reader-visible change: [^[:space:]]' | head -1 || true)

{
	printf '## apidiff\n\nExported API changes against `%s`:\n\n```\n' "$BASE_REF"
	cat "$tmp/report.txt"
	printf '```\n\n'
} >"$REPORT"
cat "$tmp/report.txt"

if [ -n "$docs_changed" ]; then
	printf 'Docs gate passed: the change touches docs.\n' | tee -a "$REPORT"
	printf '%s\n' "$docs_changed"
	exit 0
fi
if [ -n "$escape" ]; then
	printf 'Docs gate passed by the escape line: `%s`\n' "$escape" | tee -a "$REPORT"
	exit 0
fi
{
	printf 'Docs gate failed: the exported API changed but no docs changed.\n\n'
	printf 'Touch README.md, docs/**, llms.txt or examples/**, or add a line\n'
	printf '`Docs: no reader-visible change: <reason>` to the PR body or a commit message.\n'
} | tee -a "$REPORT" >&2
exit 1
