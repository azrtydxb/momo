# SPDX-License-Identifier: Apache-2.0
"""MkDocs hook: point links that leave docs/ at the repository on GitHub.

The pages link to ../examples/..., ../README.md and similar so that they work
when read in the repository. On the site those targets do not exist, so the
links are rewritten to the matching GitHub URL at build time. Links inside
fenced code blocks are left alone. A build for a version tag links to that tag
instead of main, so a versioned site points at the matching sources.
"""

import os
import re

_REPO = "https://github.com/azrtydxb/momo"
# ](../path) or ](../path "title"), also with a #fragment on the path.
_LINK = re.compile(r"\]\(((?:\.\./)+)([^)\s]+)((?:\s+\"[^\"]*\")?)\)")
_FENCE = re.compile(r"^\s*(```|~~~)")


def _ref():
    if os.environ.get("GITHUB_REF_TYPE") == "tag" and os.environ.get("GITHUB_REF_NAME"):
        return os.environ["GITHUB_REF_NAME"]
    return "main"


def _rewrite(match):
    path = match.group(2)
    kind = "tree" if path.split("#")[0].endswith("/") else "blob"
    return f"]({_REPO}/{kind}/{_ref()}/{path}{match.group(3)})"


def on_page_markdown(markdown, page, config, files):
    out = []
    in_fence = False
    for line in markdown.split("\n"):
        if _FENCE.match(line):
            in_fence = not in_fence
        out.append(line if in_fence else _LINK.sub(_rewrite, line))
    return "\n".join(out)
