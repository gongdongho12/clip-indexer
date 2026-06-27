# Gemini Asset Request

Use this when generating replacement mock assets in Gemini or another image/video model. The model may choose the exact travel subjects as long as it returns the chosen subjects, expected tags, expected group, and a short ground-truth scene summary for each generated asset.

## Requirements

- Generate one still image or short video per scene.
- Prefer the look of stable DJI/drone/gimbal travel footage. Do not add DJI logos or brand text.
- If you choose the scene topics yourself, choose varied travel footage categories that stress different analysis groups, such as nature, food, city, train/transit, hotel, landmark, shopping, people, or airport.
- No logos, no readable text, no watermarks, no brand names.
- Keep realistic travel-footage framing with a single-take gimbal/drone feel.
- Save image outputs as PNG using exactly these names:
  - `waterfront-walk.png`
  - `market-food.png`
  - `city-night.png`
  - `train-platform.png`
- If video outputs are generated instead, export browser-playable H.264/AAC MP4,
  10 seconds or longer, with matching scene names. Still images are enough;
  `scripts/create-mock-filelist.sh` will turn them into MP4 clips.

## Prompts

### waterfront-walk.png

Create a realistic DJI-style travel clip source frame representing a morning waterfront walk: seaside boardwalk with calm blue water, low morning sun, soft clouds, and a few distant travelers walking. Stable gimbal/drone travel-footage look, wide 16:9 frame, no text, no logos, no watermark.

Expected analysis target:

- Group: `nature`
- Tags: `beach`, `waterfront`, `walking`, `coastal`, `morning`, `people`
- Summary keywords: `waterfront`, `walking`, `sea`, `boardwalk`

### market-food.png

Create a realistic DJI Osmo-style travel clip source frame representing a busy lunch market food street: covered market aisle, warm food stall lights, colorful produce, food counters, and travelers walking through. Stable gimbal travel-footage look, wide 16:9 frame, no readable signs, no logos, no watermark.

Expected analysis target:

- Group: `food`
- Tags: `market`, `food`, `lunch`, `shopping`, `street`, `restaurant`, `people`
- Summary keywords: `market`, `food`, `stalls`, `people`

### city-night.png

Create a realistic vertical DJI Osmo-style travel clip source frame representing a night city walk: downtown street at night, colorful lights, wet pavement reflections, storefront glow, and pedestrians walking. Stable handheld-gimbal travel-footage look, vertical 9:16 frame, no readable signage, no logos, no watermark.

Expected analysis target:

- Group: `city`
- Tags: `city`, `nightlife`, `walking`, `vertical`, `street`, `urban`
- Summary keywords: `night`, `city`, `walking`, `signs`

### train-platform.png

Create a realistic DJI-style travel clip source frame representing an early morning train platform: clean outdoor rail platform at sunrise, train stopped beside the platform, tracks, platform markings, and commuters in the distance. Stable gimbal/drone travel-footage look, wide 16:9 frame, no readable text, no logos, no watermark.

Expected analysis target:

- Group: `train`
- Tags: `train`, `station`, `transportation`, `morning`, `platform`
- Summary keywords: `train`, `platform`, `tracks`, `morning`
