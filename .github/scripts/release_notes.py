#!/usr/bin/env python3
"""Generate GitHub Release notes from git commit subjects."""

from __future__ import annotations

import re
import os
import subprocess
import sys
from pathlib import Path


CONVENTIONAL_COMMIT = re.compile(
    r"^(?P<type>[a-z]+)(?:\([^)]+\))?(?P<breaking>!)?:\s*(?P<subject>.+)$"
)
STABLE_TAG = re.compile(r"^v\d+\.\d+\.\d+$")
GROUP_LABELS = {
    "feat": "Features",
    "fix": "Fixes",
    "perf": "Performance",
    "docs": "Documentation",
    "refactor": "Refactoring",
    "test": "Tests",
    "build": "Build and CI",
    "ci": "Build and CI",
    "chore": "Maintenance",
}
SECTION_ORDER = [
    "Features",
    "Fixes",
    "Performance",
    "Documentation",
    "Refactoring",
    "Tests",
    "Build and CI",
    "Maintenance",
    "Other changes",
]


def git(*args: str, check: bool = True) -> str:
    result = subprocess.run(
        ["git", *args],
        check=check,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    return result.stdout.strip()


def find_previous_stable_tag(current_tag: str) -> str:
    raw_tags = git("tag", "--merged", "HEAD", "--sort=-v:refname")
    tags = [tag for tag in raw_tags.splitlines() if STABLE_TAG.fullmatch(tag)]
    if STABLE_TAG.fullmatch(current_tag):
        tags = [tag for tag in tags if tag != current_tag]
    return tags[0] if tags else ""


def commit_subjects(previous_tag: str) -> list[str]:
    range_spec = f"{previous_tag}..HEAD" if previous_tag else "HEAD"
    raw_subjects = git("log", "--no-merges", "--pretty=format:%s", range_spec)
    return [line.strip() for line in raw_subjects.splitlines() if line.strip()]


def classify(subjects: list[str]) -> dict[str, list[str]]:
    groups = {label: [] for label in SECTION_ORDER}
    for subject in subjects:
        match = CONVENTIONAL_COMMIT.match(subject)
        if match:
            label = GROUP_LABELS.get(match.group("type"), "Other changes")
            entry = match.group("subject").strip()
            if match.group("breaking"):
                entry = f"{entry} (breaking)"
        else:
            label = "Other changes"
            entry = subject
        groups[label].append(entry)
    return groups


def render(current_tag: str, previous_tag: str, subjects: list[str]) -> str:
    sha = git("rev-parse", "HEAD")
    groups = classify(subjects)
    lines = [
        f"## Clip Atlas {current_tag}",
        "",
        f"Built from commit `{sha[:12]}`.",
    ]

    if previous_tag:
        lines.append(f"Changes since `{previous_tag}`.")
    else:
        lines.append("Changes from the available repository history.")

    lines.append("")
    for label in SECTION_ORDER:
        entries = groups[label]
        if label == "Features":
            lines.append("### Features")
            if entries:
                lines.extend(f"- {entry}" for entry in entries)
            else:
                lines.append("- No `feat:` commits were found in this range.")
            lines.append("")
            continue

        if entries:
            lines.append(f"### {label}")
            lines.extend(f"- {entry}" for entry in entries)
            lines.append("")

    lines.extend(
        [
            "### Install",
            "1. Download the archive for your platform from the release assets.",
            "2. Download `SHA256SUMS.txt` and verify the archive checksum.",
            "3. Extract the archive and place `clip-indexer` on your `PATH`.",
            "4. Check the installed binary with `clip-indexer --version`.",
            "",
            "### Quick Start",
            "- Index footage as JSON: `clip-indexer --pretty --trip \"Japan 2026\" /path/to/DCIM`.",
            "- Launch the local web UI: `clip-indexer serve --trip \"Japan 2026\" /path/to/DCIM`.",
            "- Use the auto-restarting dev server while editing: `clip-indexer dev --trip \"Japan 2026\" /path/to/DCIM`.",
            "- Generate a dry-run review bundle: `clip-indexer review --dest-root /path/to/organized /path/to/DCIM`.",
            "",
            "### Assets",
            "- Download the archive for your platform.",
            "- Verify the archive with `SHA256SUMS.txt`.",
            "",
        ]
    )
    return "\n".join(lines)


def main() -> int:
    current_tag = sys.argv[1] if len(sys.argv) > 1 else os.environ.get("GITHUB_REF_NAME", "")
    output_path = Path(sys.argv[2]) if len(sys.argv) > 2 else Path("release-notes.md")
    if not current_tag:
        print("release_notes.py needs a tag argument or GITHUB_REF_NAME.", file=sys.stderr)
        return 2

    previous_tag = find_previous_stable_tag(current_tag)
    subjects = commit_subjects(previous_tag)
    output_path.write_text(render(current_tag, previous_tag, subjects), encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
