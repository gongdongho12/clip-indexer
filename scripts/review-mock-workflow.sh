#!/usr/bin/env bash
set -euo pipefail

ROOT="tmp/mock-filelist"
OUT_DIR="$ROOT/review"
DEST_ROOT="$ROOT/organized"

scripts/create-mock-filelist.sh

env GOCACHE="${GOCACHE:-/private/tmp/clip-indexer-gocache}" \
  go run ./cmd/clip-indexer review \
    --pretty \
    --trip "Mock Trip" \
    --dest-root "$DEST_ROOT" \
    --out-dir "$OUT_DIR" \
    "$ROOT/DCIM"
