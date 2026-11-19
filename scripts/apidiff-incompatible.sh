#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
#
# Prints the incompatible changes of an apidiff report that the compatibility
# promise covers (docs/compatibility.md), one "- <name>: <what>" line each.
#
# Usage: apidiff-incompatible.sh REPORT MODULE [EXCLUDED]
#   REPORT    the text report of "apidiff -m" (an "Incompatible changes:" section
#             and a "Compatible changes:" section, each line "- <name>: <what>")
#   MODULE    the module path, for example github.com/azrtydxb/momo
#   EXCLUDED  file of module-relative package paths, one per line; "#" lines and
#             blank lines are ignored; a missing or empty file excludes nothing
#
# apidiff -m spells a name in the root package bare ("Select: removed"), a name in
# a child package with a "./" prefix ("./gen/grpc.Plugin: removed",
# "./gen/grpc.(*T).M: removed"), a removed package with the full import path
# ("package github.com/azrtydxb/momo/gen/grpc: removed") and a name in a package
# outside the module with its full import path. A line is excluded when, after
# those prefixes are removed, it starts with an excluded path followed by ".",
# "/", ":" or the end of the name; an excluded path covers its subpackages.
set -euo pipefail

report=${1:?usage: apidiff-incompatible.sh REPORT MODULE [EXCLUDED]}
mod=${2:?usage: apidiff-incompatible.sh REPORT MODULE [EXCLUDED]}
excluded=${3:-/dev/null}

awk -v mod="$mod" -v excluded="$excluded" '
BEGIN {
	while ((getline l < excluded) > 0) {
		split(l, f, /[ \t]+/)
		if (f[1] != "" && f[1] !~ /^#/) ex[f[1]] = 1
	}
}
/^Incompatible changes:/ { on = 1; next }
/^Compatible changes:/ { on = 0; next }
on {
	line = $0
	sub(/^- /, "", line)
	sub(/^package /, "", line)
	if (index(line, mod "/") == 1) {
		line = substr(line, length(mod) + 2)
	} else if (index(line, "./") == 1) {
		line = substr(line, 3)
	}
	skip = 0
	for (p in ex) {
		if (index(line, p) == 1) {
			c = substr(line, length(p) + 1, 1)
			if (c == "" || c == "." || c == "/" || c == ":" || c == " ") skip = 1
		}
	}
	if (!skip) print $0
}' "$report"
