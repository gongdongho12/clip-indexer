#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-tmp/mock-filelist}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
MANIFEST="$REPO_ROOT/fixtures/mock-filelist/manifest.json"
REPORT="${TMPDIR:-/tmp}/clip-indexer-mock-report.json"

"$SCRIPT_DIR/create-mock-filelist.sh" "$ROOT"

GOCACHE="${GOCACHE:-/private/tmp/clip-indexer-gocache}" \
  go run "$REPO_ROOT/cmd/clip-indexer" --pretty --trip "Mock Trip" "$ROOT/DCIM" > "$REPORT"

"$SCRIPT_DIR/compare-mock-analysis.py" --manifest "$MANIFEST" "$REPORT"
