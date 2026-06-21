package media

import (
	"slices"
	"testing"
)

func TestSampleSeconds(t *testing.T) {
	got := sampleSeconds(9, 2)
	if len(got) != 2 {
		t.Fatalf("expected two samples, got %v", got)
	}
	if got[0] != 3 || got[1] != 6 {
		t.Fatalf("unexpected samples: %v", got)
	}
}

func TestClamp01(t *testing.T) {
	if clamp01(-1) != 0 {
		t.Fatal("negative should clamp to zero")
	}
	if clamp01(2) != 1 {
		t.Fatal("above one should clamp to one")
	}
	if clamp01(0.4) != 0.4 {
		t.Fatal("in-range value should stay unchanged")
	}
}

func TestApplyVisionOutputAddsSceneAndLocationTags(t *testing.T) {
	item := Item{
		SourcePath:    "/tmp/source.mp4",
		Extension:     ".mp4",
		Tags:          []string{"video"},
		NameParts:     NameParts{Date: "20260603", Time: "184757", Slug: "clip", Sequence: "001"},
		FinalFileName: "20260603_184757_clip_001.mp4",
	}

	applyVisionOutput(&item, visionItemOutput{
		SourcePath:         item.SourcePath,
		Tags:               []string{"street", "walking"},
		SceneSummary:       "A street walking shot near Seoul City Hall.",
		LocationGuess:      "Seoul City Hall",
		LocationConfidence: 0.74,
		LocationLabel:      "서울 시청",
		SuggestedSlug:      "서울 시청 산책",
	}, 2, "test-model")

	for _, expected := range []string{"street", "walking", "seoul_city_hall", "서울_시청"} {
		if !slices.Contains(item.Tags, expected) {
			t.Fatalf("expected tag %q in %v", expected, item.Tags)
		}
	}
	if item.Content == nil || item.Content.SceneSummary == "" {
		t.Fatalf("expected content summary: %#v", item.Content)
	}
	if item.Location == nil || item.Location.Label != "서울 시청" {
		t.Fatalf("expected location label: %#v", item.Location)
	}
}
