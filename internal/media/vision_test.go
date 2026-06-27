package media

import (
	"math"
	"slices"
	"testing"
)

func TestSampleSeconds(t *testing.T) {
	got := sampleSeconds(10, 4)
	want := []float64{0.8, 3.6, 6.4, 9.2}
	if len(got) != len(want) {
		t.Fatalf("expected %d samples, got %v", len(want), got)
	}
	for index := range want {
		if !closeFloat(got[index], want[index]) {
			t.Fatalf("unexpected samples: got %v want %v", got, want)
		}
	}
}

func TestSampleSecondsSingleFrameUsesMiddle(t *testing.T) {
	got := sampleSeconds(9, 1)
	if len(got) != 1 {
		t.Fatalf("expected one sample, got %v", got)
	}
	if !closeFloat(got[0], 4.5) {
		t.Fatalf("expected middle sample, got %v", got)
	}
}

func TestVisionFrameCountUsesIntervalWhenConfigured(t *testing.T) {
	got := visionFrameCount(10, Config{VisionFrames: 2, VisionAdaptive: true, VisionSampleIntervalSeconds: 3})
	if got != 4 {
		t.Fatalf("expected four interval samples, got %d", got)
	}
}

func TestVisionFrameCountAdaptsShortClips(t *testing.T) {
	got := visionFrameCount(10, Config{VisionFrames: 2, VisionAdaptive: true})
	if got != 4 {
		t.Fatalf("expected short clips to use four samples, got %d", got)
	}
}

func TestVisionFrameCountAdaptsLongClipsWithLimit(t *testing.T) {
	got := visionFrameCount(120, Config{VisionFrames: 2, VisionAdaptive: true})
	if got != 8 {
		t.Fatalf("expected long clips to cap adaptive samples, got %d", got)
	}
}

func TestVisionFrameCountFallsBackToFixedFrames(t *testing.T) {
	got := visionFrameCount(10, Config{VisionFrames: 2, VisionAdaptive: false})
	if got != 2 {
		t.Fatalf("expected fixed frame count, got %d", got)
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

func closeFloat(left float64, right float64) bool {
	return math.Abs(left-right) < 0.0001
}
