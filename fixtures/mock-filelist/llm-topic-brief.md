# LLM Topic Brief For Golden Video Fixtures

Use this brief when asking Gemini or another model to propose a new mock video
fixture set from scratch.

## Goal

Create a small golden dataset for testing a travel-video indexing app. The app
will sample frames from each video, infer scene summaries, tags, location hints,
and deterministic groups, then compare the result with a manifest.

The model should choose the scene topics. Do not reuse the examples below unless
they are genuinely the strongest choices.

## Output Requirements

Choose 4 to 8 varied travel clip scenarios. Prefer categories that stress
different analysis groups:

- nature / beach / mountain / scenic
- food / restaurant / market
- city / street / night walk
- train / subway / bus / airport / transit
- hotel / room / lobby
- landmark / temple / museum / tower
- shopping / store / mall
- people / family / friends

For each scenario, produce:

1. A source image prompt for a realistic DJI/drone/gimbal-style travel clip.
2. The expected deterministic group key.
3. Expected tags.
4. Expected summary keywords.
5. A concise ground-truth scene summary.
6. A safe snake-case file stem.
7. Preferred orientation: `landscape` or `vertical`.

## Style Constraints

- Stable DJI/drone/gimbal travel footage feel.
- Single-take composition, not montage or slideshow.
- No readable text, logos, watermarks, or brand names in generated assets.
- Realistic travel footage, not illustration or advertisement.
- Include visible scene evidence for the expected tags.
- Prefer everyday travel footage over famous landmark dependency.

## JSON Shape

Return JSON only:

```json
{
  "clips": [
    {
      "file_stem": "clip_20260620_083015_waterfront_walk",
      "orientation": "landscape",
      "image_prompt": "A realistic DJI-style travel clip source frame ...",
      "ground_truth_summary": "Morning waterfront walking clip with sea, boardwalk, and passing travelers.",
      "expected_group": "nature",
      "expected_tags": ["beach", "waterfront", "walking", "coastal", "morning", "people"],
      "expected_summary_keywords": ["waterfront", "walking", "sea", "boardwalk"]
    }
  ]
}
```

After assets are generated, copy PNGs into `fixtures/mock-filelist/assets/`,
update `fixtures/mock-filelist/manifest.json`, then run:

```bash
scripts/create-mock-filelist.sh
scripts/verify-mock-filelist.sh
```
