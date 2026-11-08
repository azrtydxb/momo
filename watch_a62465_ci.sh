#!/bin/bash
# usage: watch_a62465_ci.sh <branch>
b="$1"
while true; do
	out=$(gh run list --branch "$b" --workflow ci-agent.yml --limit 1 --json databaseId,status,conclusion,headSha -q '.[0] | "\(.databaseId) \(.status) \(.conclusion) \(.headSha)"')
	echo "$(date +%T) $out"
	case "$out" in *" completed "*) break ;; esac
	sleep 200
done
