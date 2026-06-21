package media

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAnalysisCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "DJI_0001.MP4")
	if err := os.WriteFile(sourcePath, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}

	item := Item{
		SourcePath:          sourcePath,
		OriginalFileName:    "DJI_0001.MP4",
		Extension:           ".mp4",
		ShotAt:              "2026-06-03T09:47:57Z",
		DurationSeconds:     8.107,
		Tags:                []string{"video", "dji"},
		RecommendedFileName: "20260603_184757_dji_001.mp4",
		FinalFileName:       "20260603_184757_kansai_ticket_machine_001.mp4",
		Location: &LocationInfo{
			Label:      "Kansai International Airport",
			Source:     "llm_vision",
			Confidence: 0.9,
		},
		Content: &ContentInfo{
			SceneSummary:  "A traveler is using a train ticket machine.",
			LocationGuess: "Kansai International Airport, Japan",
			Tags:          []string{"ticket_machine", "train", "japan"},
			Model:         "gemini-3.1-flash-lite",
		},
		LLMNotes: "A traveler is using a train ticket machine.",
	}
	if err := saveAnalysisCache(item); err != nil {
		t.Fatalf("save cache: %v", err)
	}

	loaded := Item{
		SourcePath:          sourcePath,
		OriginalFileName:    "DJI_0001.MP4",
		Extension:           ".mp4",
		ShotAt:              "2026-06-03T09:47:57Z",
		DurationSeconds:     8.107,
		Tags:                []string{"video"},
		RecommendedFileName: "20260603_184757_dji_001.mp4",
		FinalFileName:       "20260603_184757_dji_001.mp4",
	}
	if warnings := applyAnalysisCache(&loaded); len(warnings) > 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if loaded.Content == nil || loaded.Content.SceneSummary == "" {
		t.Fatalf("expected cached content: %#v", loaded.Content)
	}
	if loaded.Location == nil || loaded.Location.Label != "Kansai International Airport" {
		t.Fatalf("expected cached location: %#v", loaded.Location)
	}
	if loaded.FinalFileName != "20260603_184757_kansai_ticket_machine_001.mp4" {
		t.Fatalf("expected cached final filename, got %s", loaded.FinalFileName)
	}
	if !containsString(loaded.Tags, "ticket_machine") {
		t.Fatalf("expected cached tags: %#v", loaded.Tags)
	}
}

func TestAnalysisCacheSkipsStaleDuration(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "DJI_0001.MP4")
	item := Item{
		SourcePath:       sourcePath,
		OriginalFileName: "DJI_0001.MP4",
		Extension:        ".mp4",
		ShotAt:           "2026-06-03T09:47:57Z",
		DurationSeconds:  8,
		Content:          &ContentInfo{SceneSummary: "cached"},
	}
	if err := saveAnalysisCache(item); err != nil {
		t.Fatalf("save cache: %v", err)
	}

	loaded := Item{
		SourcePath:       sourcePath,
		OriginalFileName: "DJI_0001.MP4",
		Extension:        ".mp4",
		ShotAt:           "2026-06-03T09:47:57Z",
		DurationSeconds:  30,
	}
	if warnings := applyAnalysisCache(&loaded); len(warnings) == 0 {
		t.Fatal("expected stale cache warning")
	}
	if loaded.Content != nil {
		t.Fatalf("stale cache should not apply content: %#v", loaded.Content)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
