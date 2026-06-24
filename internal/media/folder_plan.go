package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type folderEntry struct {
	Name         string `json:"name"`
	RelativePath string `json:"relative_path"`
	Path         string `json:"path"`
	Depth        int    `json:"depth"`
}

type plannedFolder struct {
	Folder   string `json:"folder"`
	Label    string `json:"label,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Existing bool   `json:"existing"`
	Count    int    `json:"count"`
}

type folderAssignment struct {
	SourcePath    string `json:"source_path"`
	Folder        string `json:"folder"`
	FinalFileName string `json:"final_file_name,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

type folderPlanOutput struct {
	Folders     []plannedFolder    `json:"folders"`
	Assignments []folderAssignment `json:"assignments"`
}

type folderPlanInput struct {
	ExistingFolders []string              `json:"existing_folders"`
	Items           []folderPlanItemInput `json:"items"`
}

type folderPlanItemInput struct {
	SourcePath      string     `json:"source_path"`
	FileName        string     `json:"file_name"`
	FinalFileName   string     `json:"final_file_name"`
	ShotAt          string     `json:"shot_at,omitempty"`
	Tags            []string   `json:"tags,omitempty"`
	Group           *GroupInfo `json:"group,omitempty"`
	Location        string     `json:"location,omitempty"`
	SceneSummary    string     `json:"scene_summary,omitempty"`
	AudioSummary    string     `json:"audio_summary,omitempty"`
	LocationGuess   string     `json:"location_guess,omitempty"`
	DurationSeconds float64    `json:"duration_seconds,omitempty"`
}

func listSubfolders(root string, maxDepth int) ([]folderEntry, []string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, nil, errors.New("folder root is required")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, nil, err
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return nil, nil, err
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("%s is not a directory", absRoot)
	}

	var folders []folderEntry
	var warnings []string
	var walk func(string, int)
	walk = func(current string, depth int) {
		if maxDepth > 0 && depth >= maxDepth {
			return
		}
		entries, err := os.ReadDir(current)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("could not read %s: %v", current, err))
			return
		}
		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			path := filepath.Join(current, entry.Name())
			relative, err := filepath.Rel(absRoot, path)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("could not resolve %s: %v", path, err))
				continue
			}
			folders = append(folders, folderEntry{
				Name:         entry.Name(),
				RelativePath: filepath.ToSlash(relative),
				Path:         path,
				Depth:        depth + 1,
			})
			walk(path, depth+1)
		}
	}
	walk(absRoot, 0)
	sort.Slice(folders, func(i, j int) bool {
		return folders[i].RelativePath < folders[j].RelativePath
	})
	return folders, warnings, nil
}

func planFoldersWithLLM(ctx context.Context, cfg Config, items []Item, existingFolders []folderEntry) (folderPlanOutput, error) {
	input := folderPlanInput{
		ExistingFolders: folderRelativePaths(existingFolders),
	}
	for _, item := range items {
		if item.Group == nil {
			updateItemGroup(&item)
		}
		input.Items = append(input.Items, folderPlanInputForItem(item))
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return folderPlanOutput{}, err
	}

	requestBody := map[string]any{
		"model": cfg.LLMModel,
		"messages": []map[string]string{
			{
				"role": "system",
				"content": strings.Join([]string{
					"You plan a folder structure for travel video clips.",
					"Use tags, scene summaries, audio clues, location hints, shot dates, and the provided deterministic group.",
					"Prefer existing folders only when the full relative path semantically fits the item. Do not nest unrelated clips under an existing folder just because it exists.",
					"When a new folder is needed, prefer a concise root-level sibling folder unless a nested existing folder is clearly the right parent.",
					"Folder values must be relative paths under the chosen root, never absolute, and never contain '..'.",
					"Do not create one folder per file. Group similar clips by activity, place, transit, hotel, food, landmark, nature, city, shopping, or people.",
					"Return only JSON: {\"folders\":[{\"folder\":\"...\",\"label\":\"...\",\"reason\":\"...\"}],\"assignments\":[{\"source_path\":\"...\",\"folder\":\"...\",\"final_file_name\":\"...\",\"reason\":\"...\"}]}",
				}, " "),
			},
			{
				"role":    "user",
				"content": string(payload),
			},
		},
		"temperature": 0.2,
	}

	content, err := callChatCompletion(ctx, cfg, requestBody)
	if err != nil {
		return folderPlanOutput{}, err
	}
	var output folderPlanOutput
	if err := json.Unmarshal([]byte(content), &output); err != nil {
		return folderPlanOutput{}, fmt.Errorf("could not parse folder plan JSON: %w", err)
	}
	return output, nil
}

func deterministicFolderPlan(items []Item, existingFolders []folderEntry) folderPlanOutput {
	assignments := make([]folderAssignment, 0, len(items))
	for _, item := range items {
		if item.Group == nil {
			updateItemGroup(&item)
		}
		folder := "other"
		reason := "fallback"
		if item.Group != nil {
			folder = item.Group.Folder
			reason = item.Group.Reason
		}
		if existing := matchExistingFolder(folder, existingFolders); existing != "" {
			folder = existing
			reason = "existing folder"
		}
		assignments = append(assignments, folderAssignment{
			SourcePath:    item.SourcePath,
			Folder:        folder,
			FinalFileName: item.FinalFileName,
			Reason:        reason,
		})
	}
	return completeFolderPlan(folderPlanOutput{Assignments: assignments}, items, existingFolders)
}

func completeFolderPlan(output folderPlanOutput, items []Item, existingFolders []folderEntry) folderPlanOutput {
	byPath := map[string]Item{}
	for _, item := range items {
		byPath[item.SourcePath] = item
	}
	existingSet := map[string]bool{}
	for _, folder := range existingFolders {
		existingSet[folder.RelativePath] = true
	}

	assignments := make([]folderAssignment, 0, len(items))
	seen := map[string]bool{}
	for _, assignment := range output.Assignments {
		item, ok := byPath[assignment.SourcePath]
		if !ok || seen[assignment.SourcePath] {
			continue
		}
		folder, err := cleanRelativeFolder(assignment.Folder)
		if err != nil {
			folder = fallbackFolderForItem(item)
		}
		finalName := sanitizeFinalFileName(assignment.FinalFileName, item.Extension)
		if finalName == "" {
			finalName = item.FinalFileName
		}
		assignments = append(assignments, folderAssignment{
			SourcePath:    assignment.SourcePath,
			Folder:        folder,
			FinalFileName: finalName,
			Reason:        strings.TrimSpace(assignment.Reason),
		})
		seen[assignment.SourcePath] = true
	}
	for _, item := range items {
		if seen[item.SourcePath] {
			continue
		}
		folder := fallbackFolderForItem(item)
		if existing := matchExistingFolder(folder, existingFolders); existing != "" {
			folder = existing
		}
		assignments = append(assignments, folderAssignment{
			SourcePath:    item.SourcePath,
			Folder:        folder,
			FinalFileName: item.FinalFileName,
			Reason:        "fallback",
		})
	}

	counts := map[string]int{}
	for _, assignment := range assignments {
		counts[assignment.Folder]++
	}
	folderMeta := map[string]plannedFolder{}
	for _, folder := range output.Folders {
		cleaned, err := cleanRelativeFolder(folder.Folder)
		if err != nil {
			continue
		}
		folder.Folder = cleaned
		folder.Existing = existingSet[cleaned]
		folder.Count = counts[cleaned]
		folderMeta[cleaned] = folder
	}
	for folder, count := range counts {
		meta := folderMeta[folder]
		if meta.Folder == "" {
			meta = plannedFolder{
				Folder:   folder,
				Label:    folderLabel(folder),
				Existing: existingSet[folder],
			}
		}
		meta.Count = count
		folderMeta[folder] = meta
	}

	folders := make([]plannedFolder, 0, len(folderMeta))
	for _, folder := range folderMeta {
		if folder.Count == 0 {
			continue
		}
		folders = append(folders, folder)
	}
	sort.Slice(folders, func(i, j int) bool {
		return folders[i].Folder < folders[j].Folder
	})
	sort.Slice(assignments, func(i, j int) bool {
		if assignments[i].Folder == assignments[j].Folder {
			return assignments[i].SourcePath < assignments[j].SourcePath
		}
		return assignments[i].Folder < assignments[j].Folder
	})
	return folderPlanOutput{Folders: folders, Assignments: assignments}
}

func cleanRelativeFolder(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("folder is required")
	}
	if strings.ContainsRune(value, 0) || filepath.IsAbs(value) {
		return "", errors.New("folder must be a relative path")
	}
	cleaned := filepath.Clean(filepath.FromSlash(value))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", errors.New("folder cannot escape the root")
	}
	parts := strings.Split(cleaned, string(filepath.Separator))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." || part == ".." {
			return "", errors.New("folder contains an unsafe path segment")
		}
	}
	return filepath.ToSlash(cleaned), nil
}

func folderPlanInputForItem(item Item) folderPlanItemInput {
	input := folderPlanItemInput{
		SourcePath:      item.SourcePath,
		FileName:        filepath.Base(item.SourcePath),
		FinalFileName:   item.FinalFileName,
		ShotAt:          item.ShotAt,
		Tags:            item.Tags,
		Group:           item.Group,
		DurationSeconds: item.DurationSeconds,
	}
	if item.Location != nil {
		input.Location = strings.TrimSpace(strings.Join([]string{item.Location.Label, item.Location.Notes}, " "))
	}
	if item.Content != nil {
		input.SceneSummary = item.Content.SceneSummary
		input.AudioSummary = item.Content.AudioSummary
		input.LocationGuess = item.Content.LocationGuess
	}
	return input
}

func fallbackFolderForItem(item Item) string {
	if item.Group == nil {
		updateItemGroup(&item)
	}
	if item.Group != nil && item.Group.Folder != "" {
		return item.Group.Folder
	}
	return "other"
}

func folderRelativePaths(folders []folderEntry) []string {
	paths := make([]string, 0, len(folders))
	for _, folder := range folders {
		paths = append(paths, folder.RelativePath)
	}
	return paths
}

func matchExistingFolder(folder string, existingFolders []folderEntry) string {
	target := slugify(folder)
	if target == "" {
		return ""
	}
	for _, existing := range existingFolders {
		if slugify(filepath.Base(existing.RelativePath)) == target {
			return existing.RelativePath
		}
	}
	for _, existing := range existingFolders {
		if strings.Contains(slugify(existing.RelativePath), target) {
			return existing.RelativePath
		}
	}
	return ""
}

func folderLabel(folder string) string {
	base := filepath.Base(filepath.FromSlash(folder))
	base = strings.ReplaceAll(base, "_", " ")
	base = strings.ReplaceAll(base, "-", " ")
	return strings.TrimSpace(base)
}
