package media

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildMermaidMindmapIncludesGroupsTagsPlacesAndClips(t *testing.T) {
	items := []Item{
		{
			SourcePath: "/tmp/clip_20260620_083015_waterfront_walk.mp4",
			Tags:       []string{"video", "beach", "walking"},
			Group:      &GroupInfo{Key: "nature", Label: "Nature", Folder: "nature"},
			Content:    &ContentInfo{LocationGuess: "Odaiba waterfront"},
		},
	}

	got := buildMermaidMindmap(items)
	for _, want := range []string{
		"mindmap",
		"root((Clip Atlas 1))",
		"Nature 1",
		"beach 1",
		"Odaiba waterfront 1",
		"clip_20260620_083015_waterfront_walk.mp4",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected mindmap to contain %q:\n%s", want, got)
		}
	}
}

func TestReviewItemsAndApplyRequestDetectsDuplicateTargets(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "organized")
	sourceA := filepath.Join(dir, "a.mp4")
	sourceB := filepath.Join(dir, "b.mp4")
	items := []Item{
		{
			SourcePath:       sourceA,
			OriginalFileName: "a.mp4",
			FinalFileName:    "same.mp4",
			Tags:             []string{"beach"},
		},
		{
			SourcePath:       sourceB,
			OriginalFileName: "b.mp4",
			FinalFileName:    "same.mp4",
			Tags:             []string{"beach"},
		},
	}
	assignments := []folderAssignment{
		{SourcePath: sourceA, Folder: "nature", FinalFileName: "same.mp4"},
		{SourcePath: sourceB, Folder: "nature", FinalFileName: "same.mp4"},
	}

	plans, request := reviewItemsAndApplyRequest(items, assignments, destination)
	if len(plans) != 2 {
		t.Fatalf("expected two plans, got %d", len(plans))
	}
	for _, plan := range plans {
		if !plan.Conflict || !strings.Contains(plan.ConflictReason, "duplicate target path") {
			t.Fatalf("expected duplicate target conflict, got %#v", plan)
		}
	}
	if len(request.Operations) != 0 {
		t.Fatalf("expected conflicting targets to be omitted from apply request, got %d", len(request.Operations))
	}
}
