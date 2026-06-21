package media

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestApplyOneRenamesAndWritesSidecar(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "DJI_20240615123012_0001_D.MP4")
	if err := os.WriteFile(source, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := &webServer{
		report: Report{
			Items: []Item{{
				SourcePath:          source,
				OriginalFileName:    filepath.Base(source),
				Extension:           ".mp4",
				ShotAt:              "2024-06-15T12:30:12+09:00",
				Tags:                []string{"video", "dji"},
				RecommendedFileName: "20240615_123012_dji_001.mp4",
				FinalFileName:       "20240615_123012_dji_001.mp4",
			}},
		},
	}

	result := server.applyOne(applyOperation{
		SourcePath:   source,
		FinalName:    "서울 산책.MP4",
		Tags:         []string{"여행", "evening"},
		Rename:       true,
		WriteSidecar: true,
	})

	if result.Status != "applied" {
		t.Fatalf("expected applied status, got %#v", result)
	}
	target := filepath.Join(dir, "서울_산책.mp4")
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("expected renamed file: %v", err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("expected old file to be gone, stat err=%v", err)
	}
	if _, err := os.Stat(target + ".clip-tags.json"); err != nil {
		t.Fatalf("expected sidecar file: %v", err)
	}
	item := server.report.Items[0]
	if item.SourcePath != target {
		t.Fatalf("expected source path to update, got %s", item.SourcePath)
	}
	if item.FinalFileName != "서울_산책.mp4" {
		t.Fatalf("expected sanitized final filename, got %s", item.FinalFileName)
	}
	if !slices.Contains(item.Tags, "여행") || !slices.Contains(item.Tags, "evening") {
		t.Fatalf("expected updated tags, got %v", item.Tags)
	}
}

func TestApplyOneRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.mp4")
	target := filepath.Join(dir, "target.mp4")
	if err := os.WriteFile(source, []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("target"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := &webServer{
		report: Report{
			Items: []Item{{
				SourcePath:    source,
				Extension:     ".mp4",
				Tags:          []string{"video"},
				FinalFileName: "source.mp4",
			}},
		},
	}

	result := server.applyOne(applyOperation{
		SourcePath: source,
		FinalName:  "target.mp4",
		Tags:       []string{"changed"},
		Rename:     true,
	})

	if result.Status != "failed" {
		t.Fatalf("expected failed status, got %#v", result)
	}
	if server.report.Items[0].FinalFileName != "source.mp4" {
		t.Fatalf("failed overwrite should not mutate final name")
	}
	if slices.Contains(server.report.Items[0].Tags, "changed") {
		t.Fatalf("failed overwrite should not mutate tags")
	}
}

func TestApplyOneMovesIntoGroupFolder(t *testing.T) {
	dir := t.TempDir()
	sourceDir := filepath.Join(dir, "input")
	groupRoot := filepath.Join(dir, "groups")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(sourceDir, "clip.mp4")
	if err := os.WriteFile(source, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source+analysisCacheSuffix, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := &webServer{
		report: Report{
			Items: []Item{{
				SourcePath:    source,
				Extension:     ".mp4",
				Tags:          []string{"train", "station"},
				FinalFileName: "clip.mp4",
			}},
		},
	}

	result := server.applyOne(applyOperation{
		SourcePath:  source,
		FinalName:   "station arrival.mp4",
		Tags:        []string{"train", "station"},
		MoveToGroup: true,
		GroupRoot:   groupRoot,
	})

	target := filepath.Join(groupRoot, "train", "station_arrival.mp4")
	if result.Status != "applied" {
		t.Fatalf("expected applied status, got %#v", result)
	}
	if !result.Moved || result.Group != "train" {
		t.Fatalf("expected train move result, got %#v", result)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("expected moved file: %v", err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("expected old file to be gone, stat err=%v", err)
	}
	if _, err := os.Stat(target + analysisCacheSuffix); err != nil {
		t.Fatalf("expected analysis cache sidecar to move: %v", err)
	}
	item := server.report.Items[0]
	if item.SourcePath != target {
		t.Fatalf("expected source path to update, got %s", item.SourcePath)
	}
	if item.Group == nil || item.Group.Key != "train" {
		t.Fatalf("expected train group, got %#v", item.Group)
	}
}

func TestApplyOneMovesIntoPlannedTargetFolder(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(source, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	groupRoot := filepath.Join(dir, "organized")

	server := &webServer{
		report: Report{
			Items: []Item{{
				SourcePath:    source,
				Extension:     ".mp4",
				Tags:          []string{"food", "cafe"},
				FinalFileName: "clip.mp4",
			}},
		},
	}

	result := server.applyOne(applyOperation{
		SourcePath:   source,
		FinalName:    "cafe clip.mp4",
		Tags:         []string{"food", "cafe"},
		MoveToGroup:  true,
		GroupRoot:    groupRoot,
		TargetFolder: "food/cafe",
	})

	target := filepath.Join(groupRoot, "food", "cafe", "cafe_clip.mp4")
	if result.Status != "applied" || !result.Moved {
		t.Fatalf("expected moved apply result, got %#v", result)
	}
	if result.TargetFolder != "food/cafe" {
		t.Fatalf("expected target folder in result, got %#v", result)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("expected target file: %v", err)
	}
	if server.report.Items[0].SourcePath != target {
		t.Fatalf("expected source path to update, got %s", server.report.Items[0].SourcePath)
	}
}
