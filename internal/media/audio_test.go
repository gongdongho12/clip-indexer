package media

import (
	"slices"
	"testing"
)

func TestApplyAudioOutputAddsTranscriptAndTags(t *testing.T) {
	item := Item{
		SourcePath:    "/tmp/source.mp4",
		Extension:     ".mp4",
		Tags:          []string{"video"},
		NameParts:     NameParts{Date: "20260603", Time: "184757", Slug: "clip", Sequence: "001"},
		FinalFileName: "20260603_184757_clip_001.mp4",
	}

	applyAudioOutput(&item, audioItemOutput{
		SourcePath:         item.SourcePath,
		Tags:               []string{"announcement", "train"},
		AudioSummary:       "A station announcement mentions Seoul Station.",
		LocationGuess:      "Seoul Station",
		LocationConfidence: 0.8,
		LocationLabel:      "서울역",
		SuggestedSlug:      "서울역 안내방송",
	}, "Next stop is Seoul Station.", 30, Config{
		AudioModel: "test-audio",
	})

	for _, expected := range []string{"speech", "announcement", "train", "seoul_station", "서울역"} {
		if !slices.Contains(item.Tags, expected) {
			t.Fatalf("expected tag %q in %v", expected, item.Tags)
		}
	}
	if item.Content == nil || item.Content.AudioTranscript != "Next stop is Seoul Station." {
		t.Fatalf("expected audio transcript: %#v", item.Content)
	}
	if item.Content.AudioSummary == "" {
		t.Fatalf("expected audio summary: %#v", item.Content)
	}
	if item.Location == nil || item.Location.Label != "서울역" {
		t.Fatalf("expected location label: %#v", item.Location)
	}
	if item.FinalFileName != "20260603_184757_서울역_안내방송_001.mp4" {
		t.Fatalf("unexpected final filename: %s", item.FinalFileName)
	}
}
