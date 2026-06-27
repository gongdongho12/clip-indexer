package media

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"time"
)

const analysisCacheSuffix = ".clip-analysis.json"

type analysisCache struct {
	Service             ServiceInfo   `json:"service"`
	CacheVersion        int           `json:"cache_version"`
	UpdatedAt           string        `json:"updated_at"`
	SourcePath          string        `json:"source_path"`
	OriginalFileName    string        `json:"original_file_name"`
	ShotAt              string        `json:"shot_at,omitempty"`
	DurationSeconds     float64       `json:"duration_seconds,omitempty"`
	Tags                []string      `json:"tags,omitempty"`
	Location            *LocationInfo `json:"location,omitempty"`
	Content             *ContentInfo  `json:"content,omitempty"`
	Group               *GroupInfo    `json:"group,omitempty"`
	RecommendedFileName string        `json:"recommended_file_name,omitempty"`
	FinalFileName       string        `json:"final_file_name,omitempty"`
	LLMNotes            string        `json:"llm_notes,omitempty"`
}

func analysisCachePath(sourcePath string) string {
	return sourcePath + analysisCacheSuffix
}

func removeAnalysisCache(sourcePath string) (bool, error) {
	path := analysisCachePath(sourcePath)
	if os.Getenv("CLIP_INDEXER_SAVE_REAL") == "1" {
		path = path + ".real"
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func applyAnalysisCache(item *Item) []string {
	if os.Getenv("CLIP_INDEXER_SAVE_REAL") == "1" {
		return nil
	}
	path := analysisCachePath(item.SourcePath)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return []string{fmt.Sprintf("analysis cache read failed for %s: %v", item.SourcePath, err)}
	}

	var cache analysisCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return []string{fmt.Sprintf("analysis cache parse failed for %s: %v", item.SourcePath, err)}
	}
	if staleReason := staleAnalysisCacheReason(*item, cache); staleReason != "" {
		return []string{fmt.Sprintf("analysis cache skipped for %s: %s", item.SourcePath, staleReason)}
	}

	item.Tags = mergeTagList(item.Tags, cache.Tags)
	if cache.Location != nil {
		location := *cache.Location
		item.Location = &location
		item.Tags = mergeTagList(item.Tags, locationTags(item.Location))
	}
	if cache.Content != nil {
		content := *cache.Content
		content.Tags = append([]string{}, cache.Content.Tags...)
		content.AudioTags = append([]string{}, cache.Content.AudioTags...)
		item.Content = &content
		item.Tags = mergeTagList(item.Tags, contentTags(item.Content))
	}
	if cache.FinalFileName != "" {
		if finalName := sanitizeFinalFileName(cache.FinalFileName, item.Extension); finalName != "" {
			item.FinalFileName = finalName
		}
	}
	if cache.Group != nil {
		group := *cache.Group
		item.Group = &group
	}
	if cache.LLMNotes != "" {
		item.LLMNotes = cache.LLMNotes
	}
	return nil
}

func staleAnalysisCacheReason(item Item, cache analysisCache) string {
	if cache.CacheVersion != 1 {
		return "unsupported cache version"
	}
	if cache.OriginalFileName != "" && cache.OriginalFileName != item.OriginalFileName {
		return "original filename changed"
	}
	if cache.ShotAt != "" && item.ShotAt != "" && cache.ShotAt != item.ShotAt {
		return "shot date changed"
	}
	if cache.DurationSeconds > 0 && item.DurationSeconds > 0 && math.Abs(cache.DurationSeconds-item.DurationSeconds) > 0.75 {
		return "duration changed"
	}
	return ""
}

func saveAnalysisCache(item Item) error {
	if item.Content == nil && item.Location == nil && item.LLMNotes == "" {
		return nil
	}
	cache := analysisCache{
		Service:             ServiceInfo{Name: serviceName, CLI: cliName, Version: version},
		CacheVersion:        1,
		UpdatedAt:           time.Now().Format(time.RFC3339),
		SourcePath:          item.SourcePath,
		OriginalFileName:    item.OriginalFileName,
		ShotAt:              item.ShotAt,
		DurationSeconds:     item.DurationSeconds,
		Tags:                item.Tags,
		Location:            item.Location,
		Content:             item.Content,
		Group:               item.Group,
		RecommendedFileName: item.RecommendedFileName,
		FinalFileName:       item.FinalFileName,
		LLMNotes:            item.LLMNotes,
	}
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	path := analysisCachePath(item.SourcePath)
	if os.Getenv("CLIP_INDEXER_SAVE_REAL") == "1" {
		path = path + ".real"
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func saveAnalysisCaches(items []Item) []string {
	var warnings []string
	for _, item := range items {
		if err := saveAnalysisCache(item); err != nil {
			warnings = append(warnings, fmt.Sprintf("analysis cache write failed for %s: %v", item.SourcePath, err))
		}
	}
	return warnings
}
