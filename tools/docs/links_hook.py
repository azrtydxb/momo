# SPDX-License-Identifier: Apache-2.0
"""MkDocs hook: point links that leave docs/ at the repository on GitHub.

The pages link to ../examples/..., ../README.md and similar so that they work
when read in the repository. On the site those targets do not exist, so the
links are rewritten to the matching GitHub URL at build time.
"""

import re

_REPO = "https://github.com/azrtydxb/momo"
_LINK = re.compile(r"\]\(((?:\.\./)+)([^)\s]+)\)")


def _rewrite(match):
    path = match.group(2)
    kind = "tree" if path.endswith("/") else "blob"
    return f"]({_REPO}/{kind}/main/{path})"


def on_page_markdown(markdown, page, config, files):
    return _LINK.sub(_rewrite, markdown)
