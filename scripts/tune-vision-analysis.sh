#!/usr/bin/env bash
set -euo pipefail

if [[ "${RUN_LIVE_LLM:-}" != "1" ]]; then
  cat >&2 <<'TEXT'
Refusing to run live LLM tuning by default.

This script runs multiple --llm-vision combinations and may spend API credits.
Set RUN_LIVE_LLM=1 after confirming LLM_API_KEY/OPENAI_API_KEY and LLM_MODEL/OPENAI_MODEL.
TEXT
  exit 1
fi

ROOT="${1:-tmp/mock-filelist}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
PROMPT_DIR="${PROMPT_DIR:-$REPO_ROOT/fixtures/mock-filelist/vision-prompts}"
OUT_DIR="${OUT_DIR:-$REPO_ROOT/tmp/vision-tuning}"
INTERVALS="${VISION_TUNE_INTERVALS:-2 3 5 10}"

mkdir -p "$OUT_DIR"
"$SCRIPT_DIR/create-mock-filelist.sh" "$ROOT"

printf "prompt,interval_seconds,score,total_frames,frame_efficiency,report\n" > "$OUT_DIR/results.csv"

for prompt in "$PROMPT_DIR"/*.md; do
  prompt_name="$(basename "$prompt" .md)"
  for interval in $INTERVALS; do
    run_dir="$OUT_DIR/${prompt_name}_every_${interval}s"
    report="$run_dir/report.json"
    score_json="$run_dir/score.json"
    mkdir -p "$run_dir"

    find "$ROOT/DCIM" -name "*.clip-analysis.json" -delete

    GOCACHE="${GOCACHE:-/private/tmp/clip-indexer-gocache}" \
      go run "$REPO_ROOT/cmd/clip-indexer" \
      --pretty \
      --trip "Mock Trip" \
      --llm-vision \
      --vision-prompt-file "$prompt" \
      --vision-sample-interval "$interval" \
      "$ROOT/DCIM" > "$report"

    "$SCRIPT_DIR/compare-mock-analysis.py" --json "$report" > "$score_json" || true

    python3 - "$prompt_name" "$interval" "$score_json" "$report" <<'PY' >> "$OUT_DIR/results.csv"
import json
import sys

prompt_name, interval, score_path, report_path = sys.argv[1:5]
with open(score_path, "r", encoding="utf-8") as fh:
    score = json.load(fh)
print(
    f"{prompt_name},{interval},{score['score']},{score['total_frames']},"
    f"{score['frame_efficiency']},{report_path}"
)
PY
  done
done

python3 - "$OUT_DIR/results.csv" <<'PY'
import csv
import sys

with open(sys.argv[1], newline="", encoding="utf-8") as fh:
    rows = list(csv.DictReader(fh))

rows.sort(key=lambda row: (float(row["score"]), float(row["frame_efficiency"])), reverse=True)
print("Best vision-analysis candidates:")
for row in rows[:5]:
    print(
        f"- prompt={row['prompt']} interval={row['interval_seconds']}s "
        f"score={row['score']} frames={row['total_frames']} "
        f"eff={row['frame_efficiency']}"
    )
PY
