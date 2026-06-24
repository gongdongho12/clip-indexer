package media

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListSubfoldersIncludesNestedFolders(t *testing.T) {
	dir := t.TempDir()
	for _, relative := range []string{"train", filepath.Join("train", "station"), "food"} {
		if err := os.MkdirAll(filepath.Join(dir, relative), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	folders, warnings, err := listSubfolders(dir, 2)
	if err != nil {
		t.Fatalf("list folders: %v", err)
	}
	if len(warnings) > 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	for _, expected := range []string{"food", "train", "train/station"} {
		if !hasFolder(folders, expected) {
			t.Fatalf("expected folder %s in %#v", expected, folders)
		}
	}
}

func TestListSubfoldersDepthZeroIsUnlimited(t *testing.T) {
	dir := t.TempDir()
	deep := filepath.Join(dir, "a", "b", "c", "d", "e")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}

	folders, warnings, err := listSubfolders(dir, 0)
	if err != nil {
		t.Fatalf("list folders: %v", err)
	}
	if len(warnings) > 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if !hasFolder(folders, "a/b/c/d/e") {
		t.Fatalf("expected unlimited depth folder in %#v", folders)
	}
}

func TestDeterministicFolderPlanUsesTagsAndExistingFolders(t *testing.T) {
	existing := []folderEntry{{RelativePath: "01_train", Name: "01_train"}}
	items := []Item{{
		SourcePath:    "/tmp/clip.mp4",
		Extension:     ".mp4",
		Tags:          []string{"train", "station"},
		FinalFileName: "clip.mp4",
	}}

	plan := deterministicFolderPlan(items, existing)

	if len(plan.Assignments) != 1 {
		t.Fatalf("expected one assignment, got %#v", plan)
	}
	if plan.Assignments[0].Folder != "01_train" {
		t.Fatalf("expected existing train folder, got %#v", plan.Assignments[0])
	}
	if len(plan.Folders) != 1 || !plan.Folders[0].Existing {
		t.Fatalf("expected existing planned folder, got %#v", plan.Folders)
	}
}

func TestCleanRelativeFolderRejectsTraversal(t *testing.T) {
	if _, err := cleanRelativeFolder("../outside"); err == nil {
		t.Fatal("expected traversal to be rejected")
	}
	if got, err := cleanRelativeFolder("train/station"); err != nil || got != "train/station" {
		t.Fatalf("expected clean nested folder, got %q err=%v", got, err)
	}
}

func hasFolder(folders []folderEntry, relative string) bool {
	for _, folder := range folders {
		if folder.RelativePath == relative {
			return true
		}
	}
	return false
}
