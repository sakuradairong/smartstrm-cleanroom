#!/usr/bin/env python3
"""Check that page-scoped stylesheets do not style other pages.

Pages are lazy-loaded, so a page-scoped stylesheet only reaches the browser with
its own chunk. A class defined there but used by a page that does not import it
renders unstyled when that page is opened directly. Shared primitives belong in
App.css, which the entry chunk always loads.
"""

from __future__ import annotations

import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parent.parent
PAGES = ROOT / "web" / "src" / "pages"
APP_STYLESHEET = ROOT / "web" / "src" / "App.css"

CLASS_DEFINITION = re.compile(r"\.([A-Za-z][\w-]*)")
CLASS_USAGE = re.compile(r"className=(?:\"([^\"]*)\"|\{`([^`]*)`\})")
CLASS_TOKEN = re.compile(r"^[A-Za-z][\w-]*$")


def defined_classes(css: str) -> set[str]:
    return set(CLASS_DEFINITION.findall(css))


def used_classes(source: str) -> set[str]:
    names: set[str] = set()
    for literal, template in CLASS_USAGE.findall(source):
        for token in re.split(r"[\s${}?:'\"]+", literal or template):
            if CLASS_TOKEN.match(token):
                names.add(token)
    return names


def main() -> int:
    shared = defined_classes(APP_STYLESHEET.read_text(encoding="utf-8"))
    sources = {
        path: path.read_text(encoding="utf-8")
        for path in sorted(PAGES.rglob("*.tsx"))
        if ".test." not in path.name
    }

    failures: list[str] = []
    for stylesheet in sorted(PAGES.rglob("*.css")):
        owner = stylesheet.with_suffix(".tsx")
        defined = defined_classes(stylesheet.read_text(encoding="utf-8"))
        for path, source in sources.items():
            if path == owner or stylesheet.name in source:
                continue
            for name in sorted(used_classes(source) & defined - shared):
                failures.append(
                    f"{path.relative_to(ROOT)}: uses .{name} defined only in "
                    f"{stylesheet.relative_to(ROOT)}; move it to web/src/App.css"
                )

    if failures:
        print("Page stylesheet scope check failed:", file=sys.stderr)
        print("\n".join(f"- {failure}" for failure in failures), file=sys.stderr)
        return 1

    print(f"Page stylesheet scope check passed ({len(sources)} pages).")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
