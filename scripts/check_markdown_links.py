#!/usr/bin/env python3
"""Check that relative links in repository Markdown files resolve."""

from __future__ import annotations

import re
import sys
from pathlib import Path
from urllib.parse import unquote


ROOT = Path(__file__).resolve().parent.parent
LINK = re.compile(r"(?<!!)\[[^\]]*\]\(([^)]+)\)")
SKIP_PREFIXES = ("#", "http://", "https://", "mailto:", "data:")


def markdown_files() -> list[Path]:
    excluded = {".git", "runtime", "artifacts"}
    return sorted(
        path
        for path in ROOT.rglob("*.md")
        if not any(part in excluded for part in path.relative_to(ROOT).parts)
    )


def link_target(raw: str) -> str:
    value = raw.strip()
    if value.startswith("<") and ">" in value:
        value = value[1 : value.index(">")]
    elif " " in value:
        value = value.split(" ", 1)[0]
    return unquote(value.split("#", 1)[0])


def main() -> int:
    failures: list[str] = []
    checked = 0
    for document in markdown_files():
        text = document.read_text(encoding="utf-8")
        for match in LINK.finditer(text):
            raw = match.group(1).strip()
            if raw.lower().startswith(SKIP_PREFIXES):
                continue
            target = link_target(raw)
            if not target:
                continue
            checked += 1
            resolved = (document.parent / target).resolve()
            try:
                resolved.relative_to(ROOT)
            except ValueError:
                failures.append(f"{document.relative_to(ROOT)}: link escapes repository: {raw}")
                continue
            if not resolved.exists():
                line = text.count("\n", 0, match.start()) + 1
                failures.append(f"{document.relative_to(ROOT)}:{line}: missing {raw}")

    if failures:
        print("Markdown link check failed:", file=sys.stderr)
        print("\n".join(f"- {failure}" for failure in failures), file=sys.stderr)
        return 1
    print(f"Markdown link check passed ({checked} relative links).")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
