#!/usr/bin/env python3
"""Audit that files mentioning legacy also contain an uppercase LEGACY comment."""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path


COMMENT_EXTENSIONS = {
    ".c",
    ".cpp",
    ".go",
    ".h",
    ".hs",
    ".html",
    ".js",
    ".lua",
    ".md",
    ".py",
    ".rs",
    ".sql",
    ".ts",
    ".tsx",
    ".yaml",
    ".yml",
}

IGNORED_PARTS = {
    ".git",
    "build",
    "diagnostic",
    "dist",
    "node_modules",
    "target",
}

PY_DOCSTRING_RE = re.compile(r'^(\s*)("""|\'\'\')')
COMMENT_PREFIX_RE = re.compile(r'^\s*(//|/\*|\*|--|#|<!--)')


def is_ignored(path: Path) -> bool:
    return any(part in IGNORED_PARTS for part in path.parts)


def python_has_legacy_comment(lines: list[str]) -> bool:
    in_docstring = False
    doc_delim: str | None = None
    for line in lines:
        stripped = line.lstrip()
        if not in_docstring and stripped.startswith("#") and "LEGACY" in line:
            return True
        match = PY_DOCSTRING_RE.match(line)
        if match:
            delim = match.group(2)
            if line.count(delim) >= 2 and line.rstrip().endswith(delim) and line.strip() != delim:
                if "LEGACY" in line:
                    return True
                continue
            if not in_docstring:
                in_docstring = True
                doc_delim = delim
                if "LEGACY" in line:
                    return True
            elif delim == doc_delim:
                if "LEGACY" in line:
                    return True
                in_docstring = False
                doc_delim = None
            continue
        if in_docstring and "LEGACY" in line:
            return True
    return False


def file_has_legacy_comment(path: Path) -> bool:
    try:
        text = path.read_text(errors="ignore")
    except Exception:
        return True

    if "legacy" not in text.lower():
        return True

    lines = text.splitlines()
    if path.suffix == ".py":
        return python_has_legacy_comment(lines)

    return any("LEGACY" in line and COMMENT_PREFIX_RE.match(line) for line in lines)


def iter_candidate_files(root: Path) -> list[Path]:
    candidates: list[Path] = []
    for path in root.rglob("*"):
        if not path.is_file():
            continue
        if is_ignored(path):
            continue
        if path.suffix not in COMMENT_EXTENSIONS:
            continue
        candidates.append(path)
    return candidates


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[1])
    args = parser.parse_args()

    root = args.root.resolve()
    violations = [
        path.relative_to(root)
        for path in iter_candidate_files(root)
        if not file_has_legacy_comment(path)
    ]

    for rel_path in violations:
        print(rel_path)

    print(f"COUNT={len(violations)}")
    return 1 if violations else 0


if __name__ == "__main__":
    sys.exit(main())
