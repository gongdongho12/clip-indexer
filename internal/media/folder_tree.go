package media

import (
	"path/filepath"
	"sort"
	"strings"
)

type folderTreeBuilder struct {
	name     string
	path     string
	files    []string
	children map[string]*folderTreeBuilder
}

func refreshReportDerived(report *Report, filesDiscovered int) {
	report.Summary = summarize(report.Items, filesDiscovered, len(report.Warnings))
	report.FolderTree = buildFolderTree(report.Items)
}

func reportFilesDiscovered(report Report) int {
	if report.Summary.FilesDiscovered > 0 {
		return report.Summary.FilesDiscovered
	}
	return len(report.Items)
}

func buildFolderTree(items []Item) []FolderTreeNode {
	filesByDir := map[string][]string{}
	dirs := make([]string, 0, len(items))
	seenDirs := map[string]bool{}
	for _, item := range items {
		sourcePath := strings.TrimSpace(item.SourcePath)
		if sourcePath == "" {
			continue
		}
		cleanedPath := filepath.Clean(sourcePath)
		dir := filepath.Dir(cleanedPath)
		filesByDir[dir] = append(filesByDir[dir], cleanedPath)
		if !seenDirs[dir] {
			dirs = append(dirs, dir)
			seenDirs[dir] = true
		}
	}
	if len(dirs) == 0 {
		return nil
	}

	rootPath := commonDirectory(dirs)
	if rootPath == "" {
		rootPath = dirs[0]
	}
	root := &folderTreeBuilder{
		name:     folderTreeName(rootPath),
		path:     rootPath,
		children: map[string]*folderTreeBuilder{},
	}

	sort.Strings(dirs)
	for _, dir := range dirs {
		files := append([]string{}, filesByDir[dir]...)
		sort.Strings(files)
		node := root
		if rel, err := filepath.Rel(rootPath, dir); err == nil && rel != "." {
			currentPath := rootPath
			for _, segment := range splitFolderTreeRelativePath(rel) {
				currentPath = filepath.Join(currentPath, segment)
				if node.children == nil {
					node.children = map[string]*folderTreeBuilder{}
				}
				child := node.children[segment]
				if child == nil {
					child = &folderTreeBuilder{
						name:     segment,
						path:     currentPath,
						children: map[string]*folderTreeBuilder{},
					}
					node.children[segment] = child
				}
				node = child
			}
		}
		node.files = append(node.files, files...)
	}

	return []FolderTreeNode{root.toNode(rootPath, 0)}
}

func splitFolderTreeRelativePath(path string) []string {
	path = filepath.Clean(path)
	if path == "." {
		return nil
	}
	segments := strings.Split(filepath.ToSlash(path), "/")
	result := make([]string, 0, len(segments))
	for _, segment := range segments {
		if segment != "" && segment != "." {
			result = append(result, segment)
		}
	}
	return result
}

func folderTreeName(path string) string {
	name := filepath.Base(path)
	if name == "." || name == string(filepath.Separator) || name == "" {
		return path
	}
	return name
}

func (node *folderTreeBuilder) toNode(rootPath string, depth int) FolderTreeNode {
	children := make([]FolderTreeNode, 0, len(node.children))
	keys := make([]string, 0, len(node.children))
	for key := range node.children {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	total := len(node.files)
	for _, key := range keys {
		child := node.children[key].toNode(rootPath, depth+1)
		total += child.TotalFileCount
		children = append(children, child)
	}

	relativePath := ""
	if rel, err := filepath.Rel(rootPath, node.path); err == nil && rel != "." {
		relativePath = filepath.ToSlash(rel)
	}
	files := append([]string{}, node.files...)
	return FolderTreeNode{
		Name:           node.name,
		Path:           node.path,
		RelativePath:   relativePath,
		Depth:          depth,
		FileCount:      len(files),
		TotalFileCount: total,
		Files:          files,
		Children:       children,
	}
}
