# SPDX-License-Identifier: Apache-2.0
"""MkDocs hook: writes llms.txt, llms-full.txt and a raw Markdown twin per page.

The links in llms.txt are relative, so the file is valid under every version
directory that mike publishes (/dev/, /v0.3/, /latest/).
"""

import os
import shutil

_pages = []


def on_nav(nav, config, files):
    global _pages
    _pages = [(p.title or p.file.src_uri, p.file.src_uri) for p in nav.pages]
    return nav


def on_post_build(config, **kwargs):
    site = config["site_dir"]
    docs = config["docs_dir"]

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
        source = os.path.join(docs, src)
        with open(source, encoding="utf-8") as f:
            full.append(f"# Source: {src}\n\n{f.read()}\n")
        twin = os.path.join(site, src)
        os.makedirs(os.path.dirname(twin), exist_ok=True)
        shutil.copyfile(source, twin)

    with open(os.path.join(site, "llms.txt"), "w", encoding="utf-8") as f:
        f.write("\n".join(lines) + "\n")
    with open(os.path.join(site, "llms-full.txt"), "w", encoding="utf-8") as f:
        f.write("\n".join(full))
