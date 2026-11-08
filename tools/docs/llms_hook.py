# SPDX-License-Identifier: Apache-2.0
"""MkDocs hook: writes llms.txt, llms-full.txt and a raw Markdown twin per page.

The links in llms.txt are relative, so the file is valid under every version
directory that mike publishes (/dev/, /v0.3/, /latest/). A line of the form
--8<-- "path" (the pymdownx.snippets include) is expanded in the twins and in
llms-full.txt, so an agent reads the included text and not the directive.
"""

import os
import re

_pages = []
_SNIPPET = re.compile(r'^--8<--\s+"([^"]+)"\s*$', re.MULTILINE)


def expand_snippets(text, base="."):
    """Replace each snippet include line with the content of the named file."""

    def repl(match):
        with open(os.path.join(base, match.group(1)), encoding="utf-8") as f:
            return f.read().rstrip("\n")

    return _SNIPPET.sub(repl, text)


def on_nav(nav, config, files):
    global _pages
    _pages = [(p.title or p.file.src_uri, p.file.src_uri) for p in nav.pages]
    return nav


def on_post_build(config, **kwargs):
    site = config["site_dir"]
    docs = config["docs_dir"]
    root = os.path.dirname(os.path.abspath(config["config_file_path"]))

    lines = [
        "# momo",
        "",
        "> Go ORM with typed queries, explicit SQL and a self-contained migration engine.",
        "",
        "## Pages",
        "",
    ]
    full = []
    for title, src in _pages:
        lines.append(f"- [{title}]({src})")
        with open(os.path.join(docs, src), encoding="utf-8") as f:
            text = expand_snippets(f.read(), root)
        full.append(f"# Source: {src}\n\n{text}\n")
        twin = os.path.join(site, src)
        os.makedirs(os.path.dirname(twin), exist_ok=True)
        with open(twin, "w", encoding="utf-8") as f:
            f.write(text)

    with open(os.path.join(site, "llms.txt"), "w", encoding="utf-8") as f:
        f.write("\n".join(lines) + "\n")
    with open(os.path.join(site, "llms-full.txt"), "w", encoding="utf-8") as f:
        f.write("\n".join(full))
