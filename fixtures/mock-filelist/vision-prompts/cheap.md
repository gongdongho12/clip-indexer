Analyze the sampled travel video frames and metadata. Return JSON only with an items array.

For each item, preserve source_path exactly and provide:
- tags: concise visible scene/activity tags
- scene_summary: one short sentence
- location_guess/location_label only when visually supported
- suggested_slug or final_file_name when obvious

Prefer low-cost, high-signal observations. Do not over-describe. Do not invent exact places or coordinates.
