package media

import (
	"path/filepath"
	"testing"
)

func TestBuildFolderTreeGroupsNestedPaths(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "input")
	clip := filepath.Join(input, "a", "clip.mp4")
	photo := filepath.Join(input, "a", "sub", "photo.jpg")
	audio := filepath.Join(input, "b", "voice.m4a")

	tree := buildFolderTree([]Item{
		{SourcePath: clip},
		{SourcePath: photo},
		{SourcePath: audio},
	})
	if len(tree) != 1 {
		t.Fatalf("expected one root node, got %#v", tree)
	}
	if tree[0].Path != input || tree[0].TotalFileCount != 3 {
		t.Fatalf("unexpected root node: %#v", tree[0])
	}
	a := findFolderTreeNode(tree[0], filepath.Join(input, "a"))
	if a == nil || a.TotalFileCount != 2 || a.RelativePath != "a" {
		t.Fatalf("expected folder a with nested count, got %#v", a)
	}
	sub := findFolderTreeNode(tree[0], filepath.Join(input, "a", "sub"))
	if sub == nil || sub.FileCount != 1 || sub.RelativePath != "a/sub" {
		t.Fatalf("expected nested sub folder, got %#v", sub)
	}
}

func findFolderTreeNode(node FolderTreeNode, path string) *FolderTreeNode {
	if node.Path == path {
		return &node
	}
	for _, child := range node.Children {
		if found := findFolderTreeNode(child, path); found != nil {
			return found
		}
	}
	return nil
}
