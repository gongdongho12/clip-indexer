#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-tmp/mock-filelist}"
DURATION_SECONDS="${MOCK_CLIP_DURATION_SECONDS:-10}"
FPS="${MOCK_CLIP_FPS:-30}"
FRAMES=$((DURATION_SECONDS * FPS))
LAST_FRAME=$((FRAMES - 1))
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
ASSET_ROOT="${MOCK_ASSET_ROOT:-$REPO_ROOT/fixtures/mock-filelist/assets}"

if ! command -v ffmpeg >/dev/null 2>&1; then
  echo "ffmpeg is required to create mock videos." >&2
  exit 1
fi

for name in mountain-scenic ramen-dining alley-walk subway-platform hotel-room; do
  if [[ ! -f "$ASSET_ROOT/$name.mp4" && ! -f "$ASSET_ROOT/$name.png" ]]; then
    echo "missing mock asset (need either $name.mp4 or $name.png) under: $ASSET_ROOT" >&2
    exit 1
  fi
done

case "$ROOT" in
  tmp/mock-filelist|*/tmp/mock-filelist|*mock-filelist)
    rm -rf "$ROOT/DCIM" "$ROOT/reference" "$ROOT/organized" "$ROOT/README.md"
    ;;
  *)
    echo "Refusing to clean custom path that does not end with mock-filelist: $ROOT" >&2
    exit 1
    ;;
esac

mkdir -p \
  "$ROOT/DCIM/100MEDIA" \
  "$ROOT/DCIM/101MEDIA" \
  "$ROOT/reference" \
  "$ROOT/organized/food" \
  "$ROOT/organized/train" \
  "$ROOT/organized/city" \
  "$ROOT/organized/nature" \
  "$ROOT/organized/hotel"

ffmpeg_common=(-y -hide_banner -loglevel error)
video_encode=(-c:v libx264 -profile:v baseline -level 3.1 -preset veryfast -crf 23 -pix_fmt yuv420p -movflags +faststart)
audio_encode=(-c:a aac -b:a 96k -ar 44100 -ac 2)

make_clip() {
  local asset_base="${1%.png}" # Strip .png extension if provided
  local output="$2"
  local size="$3"
  local tone_frequency="$4"
  local creation_time="$5"
  local motion="${6:-gimbal-push}"
  local location="${7:-}"
  local zoom_size="${size/:/x}"
  local metadata=(-metadata "creation_time=$creation_time")
  local zoom_expr
  local x_expr
  local y_expr
  if [[ -n "$location" ]]; then
    metadata+=(-metadata "location=$location")
  fi

  # If a raw video file is provided, copy it directly and apply metadata without re-encoding
  if [[ -f "$ASSET_ROOT/$asset_base.mp4" ]]; then
    echo "Using raw video asset: $ASSET_ROOT/$asset_base.mp4"
    if [[ ! -f "$ASSET_ROOT/$asset_base.png" ]]; then
      echo "Extracting frame from video to $ASSET_ROOT/$asset_base.png"
      ffmpeg -y -hide_banner -loglevel error \
        -ss 00:00:00 -i "$ASSET_ROOT/$asset_base.mp4" \
        -vframes 1 -q:v 2 \
        "$ASSET_ROOT/$asset_base.png"
    fi
    ffmpeg "${ffmpeg_common[@]}" \
      -i "$ASSET_ROOT/$asset_base.mp4" \
      -c copy "${metadata[@]}" \
      "$output"
    return
  fi

  local asset="$asset_base.png"

  case "$motion" in
    drone-slide-right)
      zoom_expr="1.08+0.035*on/${LAST_FRAME}"
      x_expr="(iw-iw/zoom)*(0.10+0.75*on/${LAST_FRAME})"
      y_expr="(ih-ih/zoom)*0.52"
      ;;
    drone-slide-left)
      zoom_expr="1.09+0.030*on/${LAST_FRAME}"
      x_expr="(iw-iw/zoom)*(0.85-0.70*on/${LAST_FRAME})"
      y_expr="(ih-ih/zoom)*0.48"
      ;;
    gimbal-walk-forward)
      zoom_expr="1.04+0.080*on/${LAST_FRAME}"
      x_expr="(iw-iw/zoom)*0.50"
      y_expr="(ih-ih/zoom)*(0.58-0.18*on/${LAST_FRAME})"
      ;;
    gimbal-tilt-up)
      zoom_expr="1.06+0.045*on/${LAST_FRAME}"
      x_expr="(iw-iw/zoom)*0.50"
      y_expr="(ih-ih/zoom)*(0.72-0.45*on/${LAST_FRAME})"
      ;;
    vertical-gimbal-walk)
      zoom_expr="1.05+0.070*on/${LAST_FRAME}"
      x_expr="(iw-iw/zoom)*(0.48+0.08*sin(on/42))"
      y_expr="(ih-ih/zoom)*(0.70-0.30*on/${LAST_FRAME})"
      ;;
    gimbal-push|*)
      zoom_expr="1.05+0.060*on/${LAST_FRAME}"
      x_expr="(iw-iw/zoom)*0.50"
      y_expr="(ih-ih/zoom)*0.50"
      ;;
  esac

  ffmpeg "${ffmpeg_common[@]}" \
    -loop 1 -i "$ASSET_ROOT/$asset" \
    -f lavfi -i "sine=frequency=${tone_frequency}:duration=${DURATION_SECONDS}" \
    -map 0:v:0 -map 1:a:0 \
    -vf "scale=${size}:force_original_aspect_ratio=increase,crop=${size},zoompan=z='${zoom_expr}':x='${x_expr}':y='${y_expr}':d=${FRAMES}:s=${zoom_size}:fps=${FPS},format=yuv420p" \
    -frames:v "$FRAMES" \
    "${metadata[@]}" \
    "${video_encode[@]}" "${audio_encode[@]}" -shortest \
    "$output"
}

make_clip \
  "mountain-scenic.png" \
  "$ROOT/DCIM/100MEDIA/clip_20260623_091500_mountain_scenic.mp4" \
  "1280:720" \
  "440" \
  "2026-06-23T00:15:00Z" \
  "drone-slide-right" \
  "+35.8627+138.6963/"

make_clip \
  "ramen-dining.png" \
  "$ROOT/DCIM/100MEDIA/clip_20260623_133045_ramen_dining.mp4" \
  "1280:720" \
  "660" \
  "2026-06-23T04:30:45Z" \
  "gimbal-walk-forward"

make_clip \
  "alley-walk.png" \
  "$ROOT/DCIM/101MEDIA/clip_20260624_182000_alley_walk.mp4" \
  "720:1280" \
  "880" \
  "2026-06-24T09:20:00Z" \
  "vertical-gimbal-walk"

make_clip \
  "subway-platform.png" \
  "$ROOT/DCIM/101MEDIA/clip_20260625_110015_subway_platform.mp4" \
  "1280:720" \
  "220" \
  "2026-06-25T02:00:15Z" \
  "drone-slide-left"

make_clip \
  "hotel-room.png" \
  "$ROOT/DCIM/101MEDIA/clip_20260625_150000_hotel_room.mp4" \
  "1280:720" \
  "330" \
  "2026-06-25T06:00:00Z" \
  "gimbal-push"

cat > "$ROOT/DCIM/100MEDIA/clip_20260623_091500_mountain_scenic.mp4.clip-analysis.json" <<'JSON'
{
  "service": { "name": "Clip Atlas", "cli": "clip-indexer", "version": "mock" },
  "cache_version": 1,
  "updated_at": "2026-06-27T00:00:00+09:00",
  "original_file_name": "clip_20260623_091500_mountain_scenic.mp4",
  "tags": ["mountain", "scenic", "nature", "outdoor", "valley"],
  "location": {
    "latitude": 35.8627,
    "longitude": 138.6963,
    "label": "Yatsugatake Mountains",
    "source": "mock-cache",
    "confidence": 0.85,
    "notes": "Mock location for alpine view."
  },
  "content": {
    "scene_summary": "Scenic drone shot of green mountain peaks and a misty valley below.",
    "location_guess": "Yatsugatake Mountains",
    "location_confidence": 0.82,
    "tags": ["mountain", "scenic", "nature", "outdoor", "valley"],
    "frame_count": 2,
    "model": "mock"
  },
  "final_file_name": "20260623_091500_mock_trip_mountain_scenic_001.mp4",
  "llm_notes": "Mock cache: tests alpine scenic peaks and valley."
}
JSON

cat > "$ROOT/DCIM/100MEDIA/clip_20260623_133045_ramen_dining.mp4.clip-analysis.json" <<'JSON'
{
  "service": { "name": "Clip Atlas", "cli": "clip-indexer", "version": "mock" },
  "cache_version": 1,
  "updated_at": "2026-06-27T00:00:00+09:00",
  "original_file_name": "clip_20260623_133045_ramen_dining.mp4",
  "tags": ["food", "dining", "restaurant", "lunch", "indoor"],
  "content": {
    "scene_summary": "Cozy noodle restaurant counter dining with a steaming bowl of ramen.",
    "audio_summary": "Ambient chatter and kitchen sounds.",
    "tags": ["food", "dining", "restaurant", "lunch", "indoor"],
    "audio_tags": ["kitchen", "chatter"],
    "frame_count": 2,
    "audio_seconds": 10,
    "model": "mock",
    "audio_model": "mock"
  },
  "final_file_name": "20260623_133045_mock_trip_ramen_dining_002.mp4",
  "llm_notes": "Mock cache: ramen noodle dining."
}
JSON

cat > "$ROOT/DCIM/101MEDIA/clip_20260624_182000_alley_walk.mp4.clip-analysis.json" <<'JSON'
{
  "service": { "name": "Clip Atlas", "cli": "clip-indexer", "version": "mock" },
  "cache_version": 1,
  "updated_at": "2026-06-27T00:00:00+09:00",
  "original_file_name": "clip_20260624_182000_alley_walk.mp4",
  "tags": ["city", "street", "walking", "vertical", "urban", "nightlife"],
  "content": {
    "scene_summary": "Vertical evening walking walk through a narrow city alley lined with glowing signs, lights, windows and lanterns.",
    "location_guess": "Narrow city street alley",
    "location_confidence": 0.60,
    "tags": ["city", "street", "walking", "vertical", "urban", "nightlife"],
    "frame_count": 2,
    "model": "mock"
  },
  "final_file_name": "20260624_182000_mock_trip_alley_walk_003.mp4",
  "llm_notes": "Mock cache: evening vertical alley walk."
}
JSON

cat > "$ROOT/DCIM/101MEDIA/clip_20260625_110015_subway_platform.mp4.clip-analysis.json" <<'JSON'
{
  "service": { "name": "Clip Atlas", "cli": "clip-indexer", "version": "mock" },
  "cache_version": 1,
  "updated_at": "2026-06-27T00:00:00+09:00",
  "original_file_name": "clip_20260625_110015_subway_platform.mp4",
  "tags": ["train", "platform", "station", "transit", "indoor"],
  "content": {
    "scene_summary": "Modern indoor subway station platform with tracks and platform safety doors.",
    "location_guess": "Subway transit station",
    "location_confidence": 0.70,
    "tags": ["train", "platform", "station", "transit", "indoor"],
    "frame_count": 2,
    "model": "mock"
  },
  "final_file_name": "20260625_110015_mock_trip_subway_platform_004.mp4",
  "llm_notes": "Mock cache: subway platform and tracks."
}
JSON

cat > "$ROOT/DCIM/101MEDIA/clip_20260625_150000_hotel_room.mp4.clip-analysis.json" <<'JSON'
{
  "service": { "name": "Clip Atlas", "cli": "clip-indexer", "version": "mock" },
  "cache_version": 1,
  "updated_at": "2026-06-27T00:00:00+09:00",
  "original_file_name": "clip_20260625_150000_hotel_room.mp4",
  "tags": ["hotel", "room", "accommodation", "window", "indoor"],
  "content": {
    "scene_summary": "Cozy hotel room view showing bed and window view of the city.",
    "tags": ["hotel", "room", "accommodation", "window", "indoor"],
    "frame_count": 2,
    "model": "mock"
  },
  "final_file_name": "20260625_150000_mock_trip_hotel_room_005.mp4",
  "llm_notes": "Mock cache: modern hotel room."
}
JSON

cat > "$ROOT/reference/not-a-video.txt" <<'TEXT'
This file is intentionally unsupported and should not appear in the default file list.
TEXT

cat > "$ROOT/reference/._clip_20260623_091500_mountain_scenic.mp4" <<'TEXT'
AppleDouble sidecar placeholder. Discovery should skip this file.
TEXT

cat > "$ROOT/README.md" <<TEXT
# Mock File List

This directory is generated by \`scripts/create-mock-filelist.sh\`.

The generated videos are ${DURATION_SECONDS}s H.264/AAC MP4 files built from
AI-generated source frames in \`fixtures/mock-filelist/assets\`. Each clip uses
a continuous DJI/gimbal-style single-take motion profile, not a slideshow.

\`\`\`bash
go run ./cmd/clip-indexer --pretty --trip "Mock Trip" tmp/mock-filelist/DCIM
go run ./cmd/clip-indexer serve --port 0 --trip "Mock Trip" tmp/mock-filelist/DCIM
\`\`\`

For folder planning / organize flows, use this root:

\`\`\`text
tmp/mock-filelist/organized
\`\`\`
TEXT

echo "Created mock file list at $ROOT"
