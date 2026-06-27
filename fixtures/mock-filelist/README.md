# Mock File List Fixtures

This fixture set is a small golden dataset for Clip Atlas analysis work.

The PNG assets in `assets/` were generated from the prompts in
`manifest.json`. `scripts/create-mock-filelist.sh` turns them into 10-second
browser-playable H.264/AAC MP4 files with continuous DJI/gimbal-style single
take motion and matching `.clip-analysis.json` sidecars.

Run:

```bash
scripts/create-mock-filelist.sh
scripts/verify-mock-filelist.sh
```

Use `manifest.json` as the ground truth when tuning scene-analysis prompts,
tag extraction, grouping rules, frame sampling intervals, and folder planning.
If a better image or video asset is generated externally, replace the matching
file under `assets/` and run the verification script again.
