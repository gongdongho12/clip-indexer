---
name: auto-release-manager
description: Use when updating Clip Atlas branch-triggered releases, automatic prerelease tags, GitHub Actions release publishing, version injection, or generated release notes.
---

# Auto Release Manager

## Overview

This skill maintains the Clip Atlas release automation that turns a push to a release branch into a tagged GitHub prerelease, then lets the tag-driven release workflow build and publish CLI artifacts.

## Required Context

Before making release automation changes, inspect these files:

- `.github/workflows/branch-release.yml`
- `.github/workflows/release.yml`
- `.github/scripts/release_notes.py`
- `docs/branching-release.md`
- `internal/media/cli.go`

Load `references/release-policy.md` when changing branch names, tag formats, version injection, or release note grouping.

## Workflow

1. Identify the requested release mode.
   - Stable releases use manually pushed `vMAJOR.MINOR.PATCH` tags.
   - Automatic branch releases use the `release/auto` branch by default.
2. Keep tag creation and artifact publishing separate.
   - `Branch Auto Release` should verify the branch and push a prerelease tag.
   - `Release` should react to tags, build binaries, create checksums, generate notes, and publish GitHub Releases.
3. Keep generated versions SemVer-compatible.
   - Stable: `vX.Y.Z`.
   - Automatic prerelease: `vX.Y.Z-auto.YYYYMMDD.RUN.SHORTSHA`.
4. Preserve release note structure.
   - `feat:` commit subjects must appear under `Features`.
   - `fix:`, `ci:`, `build:`, and other Conventional Commit types should remain grouped.
5. Update docs when behavior changes.
   - README should briefly state how to trigger releases.
   - `docs/branching-release.md` should hold the detailed branch and tag policy.

## Validation

Run lightweight local validation after edits:

```bash
git diff --check
python .github/scripts/release_notes.py v0.1.0-auto.20990101.1.local release-notes.preview.md
```

Delete `release-notes.preview.md` after checking it. If Go is installed, also run:

```bash
gofmt -w internal/media/cli.go
go test ./...
```

Do not push tags, create GitHub Releases, or publish branch changes unless the user explicitly asks for that remote action.
