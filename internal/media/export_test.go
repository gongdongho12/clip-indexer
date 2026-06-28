package media

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteStaticExportIncludesFolderAndTagViews(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(source, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	report := Report{
		Service: ServiceInfo{Name: serviceName, CLI: cliName, Version: version},
		Items: []Item{{
			SourcePath:          source,
			MediaType:           mediaTypeVideo,
			OriginalFileName:    "clip.mp4",
			Extension:           ".mp4",
			DurationSeconds:     12.5,
			Video:               &VideoInfo{Codec: "h264", Width: 1920, Height: 1080, FPS: 30},
			Audio:               &AudioInfo{Codec: "aac", Channels: 2, SampleRate: 48000},
			Location:            &LocationInfo{Latitude: 37.5519, Longitude: 126.9918, Label: "N Seoul Tower", Source: "mock", Confidence: 0.91, Notes: "Landmark visible."},
			Tags:                []string{"video", "street"},
			Group:               &GroupInfo{Key: "city", Label: "City", Folder: "city", Reason: "street scene"},
			FinalFileName:       "clip.mp4",
			RecommendedFileName: "20260628_city_walk.mp4",
			Confidence:          0.82,
			LLMNotes:            "Use this clip as the walking opener.",
			Warnings:            []string{"low light"},
			Content: &ContentInfo{
				SceneSummary:       "Street video with neon signs and walking crowds.",
				AudioSummary:       "Ambient city traffic and footsteps.",
				AudioTranscript:    "We are walking toward the tower now.",
				LocationGuess:      "N Seoul Tower",
				LocationConfidence: 0.88,
				Tags:               []string{"street", "night"},
				AudioTags:          []string{"traffic", "footsteps"},
				FrameCount:         3,
				AudioSeconds:       10,
				Model:              "mock-vision",
				AudioModel:         "mock-audio",
				Notes:              "Good establishing shot.",
			},
		}},
	}
	refreshReportDerived(&report, len(report.Items))

	outputDir := filepath.Join(dir, "export")
	manifest, err := writeStaticExport(report, exportOptions{OutputDir: outputDir, IncludeMedia: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(manifest.HTMLPath); err != nil {
		t.Fatalf("expected export HTML: %v", err)
	}
	if len(manifest.Files) != 1 || manifest.Files[0].ExportPath == "" {
		t.Fatalf("expected copied media in manifest, got %#v", manifest.Files)
	}
	if _, err := os.Stat(filepath.Join(outputDir, filepath.FromSlash(manifest.Files[0].ExportPath))); err != nil {
		t.Fatalf("expected copied media file: %v", err)
	}
	data, err := os.ReadFile(manifest.HTMLPath)
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, expected := range []string{
		"folderMap",
		"tagMap",
		"Street video with neon signs",
		"Ambient city traffic",
		"Audio Transcript",
		"We are walking toward the tower now.",
		"Use this clip as the walking opener.",
		"Group Reason",
		"Raw JSON",
		"Copy JSON",
		"Descriptions",
		"data-copy-json",
		"Copy path",
		"data-copy-path",
		">Open</a>",
	} {
		if !strings.Contains(html, expected) {
			t.Fatalf("expected export HTML to contain %q", expected)
		}
	}
	var files []exportFile
	filesData, err := os.ReadFile(manifest.FilesPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(filesData, &files); err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].MediaType != mediaTypeVideo {
		t.Fatalf("unexpected files json: %#v", files)
	}
}
