#!/usr/bin/env python3
"""Compare a Clip Atlas report against the mock-filelist golden manifest."""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path


def load_json(path: Path) -> dict:
    with path.open("r", encoding="utf-8") as fh:
        return json.load(fh)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--manifest",
        default="fixtures/mock-filelist/manifest.json",
        help="Path to mock-filelist manifest JSON.",
    )
    parser.add_argument("--json", action="store_true", help="Print machine-readable score JSON.")
    parser.add_argument("report", help="Path to Clip Atlas report JSON.")
    args = parser.parse_args()

    manifest = load_json(Path(args.manifest))
    report = load_json(Path(args.report))
    clips = manifest["clips"]
    items = {item["original_file_name"]: item for item in report.get("items", [])}
    errors: list[str] = []
    total_checks = 0
    passed_checks = 0
    total_frames = 0

    indexed = report.get("summary", {}).get("files_indexed")
    if indexed != len(clips):
        errors.append(f"expected {len(clips)} indexed files, got {indexed}")
    if report.get("warnings"):
        errors.append("expected no report warnings, got: " + "; ".join(report["warnings"]))

    for clip in clips:
        name = clip["file_name"]
        item = items.get(name)
        if not item:
            errors.append(f"missing indexed item: {name}")
            continue

        duration = float(item.get("duration_seconds") or 0)
        expected_duration = manifest["clip_duration_seconds"]
        total_frames += int((item.get("content") or {}).get("frame_count") or 0)
        total_checks += 1
        if duration < expected_duration:
            errors.append(f"{name}: duration {duration} < {expected_duration}")
        else:
            passed_checks += 1
        total_checks += 1
        if item.get("extension") != ".mp4":
            errors.append(f"{name}: expected .mp4 extension, got {item.get('extension')}")
        else:
            passed_checks += 1
        total_checks += 1
        if not item.get("video"):
            errors.append(f"{name}: missing video stream")
        else:
            passed_checks += 1
        total_checks += 1
        if not item.get("audio"):
            errors.append(f"{name}: missing audio stream")
        else:
            passed_checks += 1

        group = (item.get("group") or {}).get("key")
        total_checks += 1
        if group != clip["expected_group"]:
            errors.append(f"{name}: expected group {clip['expected_group']}, got {group}")
        else:
            passed_checks += 1

        tags = set(item.get("tags") or [])
        missing_tags = [tag for tag in clip["expected_tags"] if tag not in tags]
        total_checks += len(clip["expected_tags"])
        passed_checks += len(clip["expected_tags"]) - len(missing_tags)
        if missing_tags:
            errors.append(f"{name}: missing tags {missing_tags}")

        summary = ((item.get("content") or {}).get("scene_summary") or "").lower()
        missing_words = [
            word for word in clip["expected_summary_keywords"] if word.lower() not in summary
        ]
        total_checks += len(clip["expected_summary_keywords"])
        passed_checks += len(clip["expected_summary_keywords"]) - len(missing_words)
        if missing_words:
            errors.append(f"{name}: summary missing keywords {missing_words}")

    score = passed_checks / total_checks if total_checks else 0
    result = {
        "passed": not errors,
        "score": round(score, 4),
        "passed_checks": passed_checks,
        "total_checks": total_checks,
        "total_frames": total_frames,
        "frame_efficiency": round(score / max(1, total_frames), 6),
        "errors": errors,
    }
    if errors:
        if args.json:
            print(json.dumps(result, ensure_ascii=False, indent=2))
        else:
            print("Mock analysis comparison failed:", file=sys.stderr)
            for error in errors:
                print(f"- {error}", file=sys.stderr)
        return 1

    if args.json:
        print(json.dumps(result, ensure_ascii=False, indent=2))
    else:
        print(
            f"Mock analysis comparison passed: {len(clips)} clips, "
            f"{report['summary']['files_indexed']} indexed, "
            f"{report['summary']['with_content']} with content, "
            f"score {result['score']:.4f}, frames {total_frames}."
        )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
