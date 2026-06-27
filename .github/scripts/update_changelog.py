#!/usr/bin/env python3
"""Generate a dated CHANGELOG entry from commits since the last tag.

Uses the same conventional-commit conventions as release_notes.py
(GROUP_LABELS, SECTION_ORDER, CONVENTIONAL_COMMIT regex).

Improvements over the naive approach:
- File-based intermediate storage: git log output is written to a temp
  JSON file and read back, avoiding large stdout buffer issues.
- Batched git log: commits are collected in pages (--skip / -n) so that
  repositories with thousands of commits don't hit shell/pipe limits.
- Incremental state: a .changelog-state.json file tracks the last
  processed commit SHA so repeated runs only process new commits.
- Changed-files detection uses `git diff --name-only` against a file
  list written to disk, not per-file git-log calls.
"""

from __future__ import annotations

import json
import os
import re
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

# ---------------------------------------------------------------------------
# Shared conventions (mirrored from release_notes.py)
# ---------------------------------------------------------------------------
CONVENTIONAL_COMMIT = re.compile(
    r"^(?P<type>[a-z]+)(?:\([^)]+\))?(?P<breaking>!)?:\s*(?P<subject>.+)$"
)
STABLE_TAG = re.compile(r"^v\d+\.\d+\.\d+$")

GROUP_LABELS: dict[str, str] = {
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

SECTION_ORDER: list[str] = [
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

# Paths whose changes deserve a special "Analysis & Classification" callout.
ANALYSIS_PATHS = {
    "internal/media/group.go",
    "internal/media/vision.go",
}

# Git log batch size — process this many commits per git-log invocation.
BATCH_SIZE = 500

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def git(*args: str, check: bool = True) -> str:
    """Run a git command and return its stripped stdout."""
    result = subprocess.run(
        ["git", *args],
        check=check,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    return result.stdout.strip()


def git_to_file(output_path: Path, *args: str, check: bool = True) -> int:
    """Run a git command, write stdout to *output_path*, return line count."""
    with output_path.open("w", encoding="utf-8") as fh:
        result = subprocess.run(
            ["git", *args],
            check=check,
            stdout=fh,
            stderr=subprocess.PIPE,
            text=True,
        )
    if result.returncode != 0 and check:
        raise subprocess.CalledProcessError(result.returncode, ["git", *args])
    count = sum(1 for _ in output_path.open(encoding="utf-8") if _.strip())
    return count


def find_last_tag() -> str:
    """Return the most recent stable tag reachable from HEAD, or ''."""
    raw_tags = git("tag", "--merged", "HEAD", "--sort=-v:refname", check=False)
    if not raw_tags:
        return ""
    for tag in raw_tags.splitlines():
        if STABLE_TAG.fullmatch(tag.strip()):
            return tag.strip()
    return ""


# ---------------------------------------------------------------------------
# State management — track last processed commit
# ---------------------------------------------------------------------------

STATE_FILE = ".changelog-state.json"


def load_state(work_dir: Path) -> dict[str, Any]:
    """Load the incremental state file, or return an empty dict."""
    state_path = work_dir / STATE_FILE
    if state_path.exists():
        try:
            return json.loads(state_path.read_text(encoding="utf-8"))
        except (json.JSONDecodeError, OSError):
            pass
    return {}


def save_state(work_dir: Path, state: dict[str, Any]) -> None:
    """Persist the incremental state file."""
    state_path = work_dir / STATE_FILE
    state_path.write_text(json.dumps(state, indent=2) + "\n", encoding="utf-8")


# ---------------------------------------------------------------------------
# Commit collection — batched, file-backed
# ---------------------------------------------------------------------------

def collect_commits_batched(
    since_tag: str,
    since_sha: str,
    work_dir: Path,
) -> list[dict[str, str]]:
    """Collect commits in batches, writing intermediate results to a JSON file.

    *since_sha* takes priority over *since_tag* for incremental runs.
    Falls back to *since_tag*, then to full history.
    """
    # Determine the range boundary.
    if since_sha:
        range_spec = f"{since_sha}..HEAD"
    elif since_tag:
        range_spec = f"{since_tag}..HEAD"
    else:
        range_spec = "HEAD"

    all_commits: list[dict[str, str]] = []
    offset = 0
    batch_idx = 0

    while True:
        batch_file = work_dir / f"commits_batch_{batch_idx:04d}.txt"
        count = git_to_file(
            batch_file,
            "log",
            "--no-merges",
            f"--pretty=format:%H %s",
            f"--skip={offset}",
            f"-n", str(BATCH_SIZE),
            range_spec,
            check=False,
        )

        if count == 0:
            batch_file.unlink(missing_ok=True)
            break

        # Parse the batch file.
        for line in batch_file.read_text(encoding="utf-8").splitlines():
            line = line.strip()
            if not line:
                continue
            sha, _, subject = line.partition(" ")
            if sha and subject:
                all_commits.append({"sha": sha, "subject": subject})

        batch_file.unlink(missing_ok=True)
        offset += BATCH_SIZE
        batch_idx += 1

        # Safety limit: 50,000 commits max.
        if offset >= 50_000:
            print(
                f"⚠ Reached {offset} commits, stopping pagination.",
                file=sys.stderr,
            )
            break

    # Save the full commit list to a JSON file for debugging / artifact upload.
    commits_json = work_dir / "commits_collected.json"
    commits_json.write_text(
        json.dumps(all_commits, indent=2, ensure_ascii=False) + "\n",
        encoding="utf-8",
    )
    print(
        f"  Collected {len(all_commits)} commits → {commits_json}",
        file=sys.stderr,
    )

    return all_commits


# ---------------------------------------------------------------------------
# Changed-files detection — single git-diff call
# ---------------------------------------------------------------------------

def detect_analysis_commits(
    commits: list[dict[str, str]],
    since_tag: str,
    since_sha: str,
) -> list[str]:
    """Return subjects of commits that touched Analysis & Classification files.

    Uses a single `git log` call with `-- <paths>` instead of per-file calls.
    """
    if since_sha:
        range_spec = f"{since_sha}..HEAD"
    elif since_tag:
        range_spec = f"{since_tag}..HEAD"
    else:
        range_spec = "HEAD"

    paths_args = ["--"] + sorted(ANALYSIS_PATHS)
    raw = git(
        "log", "--no-merges", "--pretty=format:%H", range_spec,
        *paths_args,
        check=False,
    )
    if not raw:
        return []

    touched_shas = {line.strip() for line in raw.splitlines() if line.strip()}

    # Match against collected commits to get subjects.
    analysis_subjects: list[str] = []
    seen: set[str] = set()
    for commit in commits:
        if commit["sha"] in touched_shas and commit["subject"] not in seen:
            analysis_subjects.append(commit["subject"])
            seen.add(commit["subject"])

    return analysis_subjects


# ---------------------------------------------------------------------------
# Classification
# ---------------------------------------------------------------------------

def classify(commits: list[dict[str, str]]) -> dict[str, list[str]]:
    """Classify commit subjects into groups (same logic as release_notes.py)."""
    groups: dict[str, list[str]] = {label: [] for label in SECTION_ORDER}
    for commit in commits:
        subject = commit["subject"]
        sha_short = commit["sha"][:10]
        match = CONVENTIONAL_COMMIT.match(subject)
        if match:
            label = GROUP_LABELS.get(match.group("type"), "Other changes")
            entry = match.group("subject").strip()
            if match.group("breaking"):
                entry = f"{entry} (**breaking**)"
        else:
            label = "Other changes"
            entry = subject
        groups[label].append(f"{entry} (`{sha_short}`)")
    return groups


# ---------------------------------------------------------------------------
# Rendering
# ---------------------------------------------------------------------------

def render_entry(
    since_tag: str,
    commits: list[dict[str, str]],
    analysis_subjects: list[str],
) -> str:
    """Build a single dated CHANGELOG entry in Markdown."""
    today = datetime.now(timezone.utc).strftime("%Y-%m-%d")
    head_sha = git("rev-parse", "HEAD")[:12]

    header = f"## {today}"
    lines: list[str] = [
        header,
        "",
        f"Built from `{head_sha}`. **{len(commits)}** commit(s) processed.",
    ]
    if since_tag:
        lines.append(f"Changes since `{since_tag}`.")
    else:
        lines.append("Changes from the full repository history.")
    lines.append("")

    groups = classify(commits)

    # Standard sections
    for label in SECTION_ORDER:
        entries = groups[label]
        if not entries:
            continue
        lines.append(f"### {label}")
        lines.extend(f"- {e}" for e in entries)
        lines.append("")

    # Special highlight: Analysis & Classification
    if analysis_subjects:
        lines.append("### 🔍 Analysis & Classification")
        lines.append("")
        lines.append(
            "The following changes affect media analysis and classification "
            "(`internal/media/group.go`, `internal/media/vision.go`):"
        )
        lines.append("")
        for subj in analysis_subjects:
            lines.append(f"- {subj}")
        lines.append("")

    return "\n".join(lines)


# ---------------------------------------------------------------------------
# CHANGELOG file management
# ---------------------------------------------------------------------------

CHANGELOG_HEADER = (
    "# Changelog\n\n"
    "All notable changes to **Clip Atlas** are documented in this file.\n\n"
)


def prepend_to_changelog(entry: str, changelog_path: Path) -> None:
    """Prepend *entry* to CHANGELOG.md, creating the file if necessary."""
    if changelog_path.exists():
        existing = changelog_path.read_text(encoding="utf-8")
        # If the file already starts with the standard header, insert after it.
        if existing.startswith(CHANGELOG_HEADER):
            body = existing[len(CHANGELOG_HEADER):]
            new_content = CHANGELOG_HEADER + entry + "\n" + body
        else:
            new_content = entry + "\n\n" + existing
    else:
        new_content = CHANGELOG_HEADER + entry + "\n"

    changelog_path.write_text(new_content, encoding="utf-8")


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main() -> int:
    changelog_path = Path(sys.argv[1] if len(sys.argv) > 1 else "CHANGELOG.md")

    # Work directory for intermediate files.
    work_dir = Path(os.environ.get("CHANGELOG_WORK_DIR", ".changelog-work"))
    work_dir.mkdir(parents=True, exist_ok=True)

    since_tag = find_last_tag()

    # Load incremental state — skip already-processed commits.
    state = load_state(work_dir)
    since_sha = state.get("last_sha", "")

    # Validate the since_sha still exists in the repo.
    if since_sha:
        verify = git("cat-file", "-t", since_sha, check=False)
        if verify != "commit":
            print(
                f"  State SHA {since_sha[:10]} not found, falling back to tag.",
                file=sys.stderr,
            )
            since_sha = ""

    commits = collect_commits_batched(since_tag, since_sha, work_dir)

    if not commits:
        print("No new commits since last tag — nothing to add.", file=sys.stderr)
        return 0

    analysis_subjects = detect_analysis_commits(commits, since_tag, since_sha)
    entry = render_entry(since_tag, commits, analysis_subjects)

    # Save rendered entry to file for artifact upload / debugging.
    entry_file = work_dir / "changelog_entry.md"
    entry_file.write_text(entry + "\n", encoding="utf-8")
    print(f"  Entry written → {entry_file}", file=sys.stderr)

    prepend_to_changelog(entry, changelog_path)

    # Save classified results as JSON for artifact upload.
    classified_file = work_dir / "classified.json"
    groups = classify(commits)
    classified_file.write_text(
        json.dumps(groups, indent=2, ensure_ascii=False) + "\n",
        encoding="utf-8",
    )

    # Update state with the newest commit SHA.
    new_head = git("rev-parse", "HEAD")
    state["last_sha"] = new_head
    state["last_run"] = datetime.now(timezone.utc).isoformat()
    state["commits_processed"] = len(commits)
    save_state(work_dir, state)

    tag_info = f" (since {since_tag})" if since_tag else ""
    sha_info = f" (incremental from {since_sha[:10]})" if since_sha else ""
    print(
        f"✓ Prepended {len(commits)} commit(s){tag_info}{sha_info} "
        f"to {changelog_path}",
        file=sys.stderr,
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
