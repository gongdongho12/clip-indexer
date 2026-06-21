# Clip Indexer

`clip-indexer` is a Go CLI for travel footage intake. Give it video files or folders and it emits JSON with:

- detected shot date
- media metadata from `ffprobe`
- tags such as device, resolution, orientation, frame rate, duration class, and time of day
- a recommended filename
- a final filename, optionally enriched by an LLM

## Service Name

My recommended service name is **Clip Atlas**.

Other names that fit the travel stack:

- **Footage Atlas**: clear, practical, more archive-oriented
- **Route Clips**: connects nicely with trip plans and route animation
- **Scene Ledger**: good if the product becomes more metadata/cost/story heavy
- **Waypoint Clips**: strong travel feeling, a little more branded
- **Clip Cartographer**: playful and map-connected, but longer

`clip-indexer` stays as the CLI/binary name so the service brand can change later.

## Requirements

- Go 1.26+
- `ffprobe` from FFmpeg in `PATH`

## Usage

```bash
go run ./cmd/clip-indexer --pretty --trip "Seoul 2026" ~/Movies/trip
```

Recursive directory scan:

```bash
go run ./cmd/clip-indexer --recursive --pretty ~/Movies/trip
```

Build a local binary:

```bash
go build -o clip-indexer ./cmd/clip-indexer
./clip-indexer --pretty ~/Movies/trip
```

## Web File Manager

Launch the local review UI:

```bash
go run ./cmd/clip-indexer serve --recursive --trip "Seoul 2026" ~/Movies/trip
```

The server prints a localhost URL. Open it in a browser to review:

- original file path
- shot date
- recommended/final filename
- editable tags
- raw JSON report
- video preview for scanned files

The UI can apply selected operations:

- analyze selected videos with LLM vision/audio when API credentials, a vision-capable model, and an audio transcription model are configured
- rename files in place
- write a sidecar tag file next to the video: `video.mp4.clip-tags.json`
- write macOS extended attributes under `com.clipatlas.tags`

The server refuses overwrites. Closing the web page or pressing **Stop server** calls the shutdown endpoint and stops the Go process.

## LLM Enrichment

The first pass works without an LLM. It uses file names, `ffprobe` metadata, and filesystem timestamps.

To let an OpenAI-compatible chat endpoint refine metadata-based tags and final filenames:

```bash
OPENAI_API_KEY="..." \
OPENAI_MODEL="your-model" \
go run ./cmd/clip-indexer --pretty --llm ~/Movies/trip
```

You can also put local credentials in `.env.local`; it is ignored by git.

```bash
OPENAI_API_KEY=...
OPENAI_MODEL=your-model
```

To analyze sampled video frames for visible scene content, cautious place guesses, and additional tags, add `--llm-vision`:

```bash
go run ./cmd/clip-indexer \
  --pretty \
  --llm-vision \
  --vision-frames 2 \
  --vision-max-items 12 \
  --trip "Seoul 2026" \
  ~/Movies/trip
```

`--llm-vision` sends sampled JPEG frames to the configured LLM provider. It is off by default because it can cost more and may include private visual content. Use `--vision-max-items 0` to analyze every indexed video.

If GPS-like tags exist in the file metadata, the JSON includes a `location` object. DJI Pocket footage often stores metadata in proprietary tracks, so plain `ffprobe` may not expose coordinates. When GPS is absent, vision analysis can still provide a cautious `content.location_guess`, but it does not invent precise coordinates.

To also extract an audio sample, transcribe speech, and turn spoken clues into tags:

```bash
go run ./cmd/clip-indexer \
  --pretty \
  --llm-vision \
  --llm-audio \
  --audio-model whisper-1 \
  --audio-max-seconds 45 \
  --vision-max-items 5 \
  --audio-max-items 5 \
  --trip "Seoul 2026" \
  ~/Movies/trip
```

In the web UI, select one or more rows and press **Analyze selected** to run frame and audio analysis on demand. The result updates the in-memory report with `content`, `location`, and merged scene/place/audio tags. It does not rename or write files unless you explicitly use the apply controls afterward.

For a local or custom endpoint:

```bash
go run ./cmd/clip-indexer \
  --llm \
  --llm-base-url http://localhost:11434/v1 \
  --llm-model your-model \
  ~/Movies/trip
```

## JSON Shape

```json
{
  "service": {
    "name": "Clip Atlas",
    "cli": "clip-indexer",
    "version": "0.1.0"
  },
  "generated_at": "2026-06-21T16:30:00+09:00",
  "items": [
    {
      "source_path": "/video/DJI_20240615123012_0001_D.MP4",
      "shot_at": "2024-06-15T12:30:12+09:00",
      "shot_at_source": "filename_datetime",
      "tags": ["video", "dji", "4k", "landscape", "60fps"],
      "recommended_file_name": "20240615_123012_seoul_dji_4k_landscape_60fps_001.mp4",
      "final_file_name": "20240615_123012_seoul_dji_4k_landscape_60fps_001.mp4"
    }
  ]
}
```
