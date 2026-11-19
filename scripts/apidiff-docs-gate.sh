#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
#
# Docs gate: an exported API change must come with a docs change.
#
# Compares the exported API of every non-internal package in the root module
# between the base and HEAD with golang.org/x/exp/cmd/apidiff. A non-empty
# report (compatible or incompatible) fails unless
#   1. the diff touches README.md, llms.txt, docs/**/*.md or examples/**/README.md, or
#   2. a commit message in the range, or the pull request body, has a line that
#      starts in column 0 with "Docs: no reader-visible change: " and a real
#      reason (not blank, not a <placeholder>).
#
# Compatibility gate (docs/compatibility.md): once the module version on the
# merge base is 1 or later (the "." entry of .release-please-manifest.json), an
# incompatible change also fails unless the change is declared breaking: the
# pull request title is a Conventional Commits title with "!" (feat!: ...; the
# type is matched case-insensitively), or the pull request body or a commit
# message has a line that starts in column 0 with "BREAKING CHANGE: " or
# "BREAKING-CHANGE: " and a description. Incompatible changes in the packages
# listed in scripts/apidiff-experimental.txt never fail it; the list and the
# manifest are both read from the merge base, so a pull request cannot edit its
# own exemption. apidiff -m names a child-package symbol "./gen/grpc.Plugin" and
# a removed package "package <full import path>"; scripts/apidiff-incompatible.sh
# strips those prefixes before it matches the list (it is tested in
# internal/repocheck). The manifest is only read when a covered package has an
# incompatible change, and an unreadable one counts as pre-v1. Before 1 the gate
# behaves as it always did: only the docs rule above applies.
#
# Environment:
#   APIDIFF   command that runs apidiff, e.g. "go run golang.org/x/exp/cmd/apidiff@<ver>" (required)
#   BASE_REF  ref to compare against; default: the origin/main or origin/sprint-* branch
#             with the fewest commits not in HEAD
#   PR_BODY_FILE  file holding the pull request body, searched for the escape line
#   PR_BODY       the body itself, if PR_BODY_FILE is not set
#   PR_TITLE_FILE file holding the pull request title, read for the "!" marker
#   PR_TITLE      the title itself, if PR_TITLE_FILE is not set
#   MODULE_VERSION  the module version to judge by; default: the "." entry of
#             .release-please-manifest.json at the merge base
#   REPORT    markdown report file (default $RUNNER_TEMP/apidiff-report.md, else
#             ./apidiff-report.md, which is git-ignored)
set -euo pipefail

: "${APIDIFF:?APIDIFF must be set}"
REPORT=${REPORT:-${RUNNER_TEMP:-.}/apidiff-report.md}
PR_BODY=${PR_BODY:-}
if [ -n "${PR_BODY_FILE:-}" ]; then
	PR_BODY=$(cat "$PR_BODY_FILE")
fi
PR_TITLE=${PR_TITLE:-}
if [ -n "${PR_TITLE_FILE:-}" ]; then
	PR_TITLE=$(cat "$PR_TITLE_FILE")
fi
MARKER='<!-- apidiff-docs -->'
rm -f "$REPORT"

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
	printf '%s\n## apidiff\n\nNo exported API change against `%s`.\n' "$MARKER" "$BASE_REF" >"$REPORT"
	exit 0
fi

docs_changed=$(git diff --name-only "$BASE_SHA" HEAD | grep -E '^(README\.md|llms\.txt|docs/.*\.md|examples/(.*/)?README\.md)$' || true)
escape=$({
	printf '%s\n' "$PR_BODY"
	git log --format=%B "$BASE_SHA..HEAD"
} | tr -d '\r' |
	grep -E '^Docs: no reader-visible change: [^[:space:]]' |
	grep -vE '^Docs: no reader-visible change: <[^>]*>[[:space:]]*$' | head -1 || true)

{
	printf '%s\n## apidiff\n\nExported API changes against `%s`:\n\n```\n' "$MARKER" "$BASE_REF"
	cat "$tmp/report.txt"
	printf '```\n\n'
} >"$REPORT"
cat "$tmp/report.txt"

# The compatibility gate only looks at incompatible changes in covered packages.
# The excluded packages and the module version both come from the merge base, not
# from the pull request, so a pull request cannot exclude a package or switch the
# gate off by editing scripts/apidiff-experimental.txt or the manifest. A file
# missing on the base (or an unreadable manifest) means nothing is excluded and
# the module is pre-v1.
git show "$BASE_SHA:scripts/apidiff-experimental.txt" >"$tmp/experimental.txt" 2>/dev/null || : >"$tmp/experimental.txt"
incompat=$(bash scripts/apidiff-incompatible.sh "$tmp/report.txt" "$MOD" "$tmp/experimental.txt")
if [ -n "$incompat" ]; then
	if [ -z "${MODULE_VERSION:-}" ]; then
		MODULE_VERSION=$(git show "$BASE_SHA:.release-please-manifest.json" 2>/dev/null |
			sed -n 's/^[[:space:]]*"\.":[[:space:]]*"\([^"]*\)".*/\1/p' | head -1 || true)
	fi
	MAJOR=${MODULE_VERSION%%.*}
	case "$MAJOR" in
	'' | *[!0-9]*)
		echo "apidiff gate: cannot read the module version from .release-please-manifest.json on the base (got '$MODULE_VERSION'); treating the module as pre-v1" >&2
		MAJOR=0
		;;
	esac
	echo "apidiff gate: module version ${MODULE_VERSION:-unknown}, incompatible changes in covered packages: yes"
fi
if [ -n "$incompat" ] && [ "$MAJOR" -ge 1 ]; then
	declared=$({
		printf '%s\n' "$PR_TITLE" | grep -iE '^[a-z]+(\([^)]*\))?!:' || true
		{
			printf '%s\n' "$PR_BODY"
			git log --format=%B "$BASE_SHA..HEAD"
		} | tr -d '\r' | grep -E '^BREAKING[ -]CHANGE: [^[:space:]]' || true
	} | head -1)
	if [ -z "$declared" ]; then
		{
			printf 'Compatibility gate failed: this module is at %s and the change is incompatible:\n\n' "$MODULE_VERSION"
			printf '%s\n\n' "$incompat"
			printf 'Keep the old API working and deprecate it (docs/compatibility.md), or declare the change\n'
			printf 'breaking: a pull request title such as `feat!: ...` or a `BREAKING CHANGE: <what>` line in\n'
			printf 'the pull request body. A breaking change ships in the next major version.\n'
		} | tee -a "$REPORT" >&2
		exit 1
	fi
	printf 'Compatibility gate passed: the incompatible change is declared breaking: `%s`\n' "$declared" | tee -a "$REPORT"
fi

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
	printf 'Touch README.md, llms.txt, docs/**/*.md or examples/**/README.md, or add a line\n'
	printf '`Docs: no reader-visible change: <reason>` to the PR body or a commit message.\n'
} | tee -a "$REPORT" >&2
exit 1
