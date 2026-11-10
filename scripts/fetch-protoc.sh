#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
#
# Installs the protoc release pinned in internal/integration/tools.lock into
# internal/integration/protoc-dist (bin/protoc and include/). The archive is
# downloaded from the protobuf releases and its SHA-256 must equal the pin in
# tools.lock, else nothing is installed.
set -euo pipefail

VERSION=36.2
root=$(cd "$(dirname "$0")/.." && pwd)
dist=$root/internal/integration/protoc-dist
lock=$root/internal/integration/tools.lock

if [ -x "$dist/bin/protoc" ] && [ "$("$dist/bin/protoc" --version)" = "libprotoc $VERSION" ]; then
	echo "protoc $VERSION is installed: $dist/bin/protoc"
	exit 0
fi

case "$(uname -s)-$(uname -m)" in
Linux-x86_64) archive=protoc-$VERSION-linux-x86_64.zip ;;
Darwin-arm64) archive=protoc-$VERSION-osx-aarch_64.zip ;;
*)
	echo "fetch-protoc: no pinned protoc archive for $(uname -s)-$(uname -m)" >&2
	exit 1
	;;
esac

want=$(awk -v a="$archive" '$1 == a { print $2 }' "$lock")
if [ -z "$want" ]; then
	echo "fetch-protoc: $archive is not in tools.lock" >&2
	exit 1
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
curl -sSfL --retry 3 -o "$tmp/$archive" "https://github.com/protocolbuffers/protobuf/releases/download/v$VERSION/$archive"
if command -v sha256sum >/dev/null 2>&1; then
	got=$(sha256sum "$tmp/$archive" | awk '{ print $1 }')
else
	got=$(shasum -a 256 "$tmp/$archive" | awk '{ print $1 }')
fi
if [ "$got" != "$want" ]; then
	echo "fetch-protoc: $archive has SHA-256 $got, tools.lock pins $want" >&2
	exit 1
fi
rm -rf "$dist"
mkdir -p "$dist"
unzip -q "$tmp/$archive" -d "$dist"
chmod +x "$dist/bin/protoc"
echo "installed protoc $VERSION: $dist/bin/protoc"
