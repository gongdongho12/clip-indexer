package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
)

type llmItemInput struct {
	SourcePath          string        `json:"source_path"`
	OriginalFileName    string        `json:"original_file_name"`
	ShotAt              string        `json:"shot_at,omitempty"`
	DurationSeconds     float64       `json:"duration_seconds,omitempty"`
	Video               *VideoInfo    `json:"video,omitempty"`
	Audio               *AudioInfo    `json:"audio,omitempty"`
	Location            *LocationInfo `json:"location,omitempty"`
	Content             *ContentInfo  `json:"content,omitempty"`
	Tags                []string      `json:"tags"`
	RecommendedFileName string        `json:"recommended_file_name"`
}

type llmOutput struct {
	Items []llmItemOutput `json:"items"`
}

type llmItemOutput struct {
	SourcePath         string   `json:"source_path"`
	Tags               []string `json:"tags,omitempty"`
	SuggestedSlug      string   `json:"suggested_slug,omitempty"`
	FinalFileName      string   `json:"final_file_name,omitempty"`
	SceneSummary       string   `json:"scene_summary,omitempty"`
	LocationGuess      string   `json:"location_guess,omitempty"`
	LocationConfidence float64  `json:"location_confidence,omitempty"`
	LocationLabel      string   `json:"location_label,omitempty"`
	Notes              string   `json:"notes,omitempty"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func EnrichWithLLM(ctx context.Context, cfg Config, items []Item) []string {
	var inputs []llmItemInput
	for _, item := range items {
		inputs = append(inputs, llmItemInput{
			SourcePath:          item.SourcePath,
			OriginalFileName:    item.OriginalFileName,
			ShotAt:              item.ShotAt,
			DurationSeconds:     item.DurationSeconds,
			Video:               item.Video,
			Audio:               item.Audio,
			Location:            item.Location,
			Content:             item.Content,
			Tags:                item.Tags,
			RecommendedFileName: item.RecommendedFileName,
		})
	}

	userPayload, err := json.Marshal(inputs)
	if err != nil {
		return []string{fmt.Sprintf("could not build LLM payload: %v", err)}
	}

	requestBody := map[string]any{
		"model": cfg.LLMModel,
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": "You rename and organize travel video clips from metadata. Return only JSON with an items array. For each input item, keep source_path exact, add concise tags, optionally provide suggested_slug, final_file_name, scene_summary, location_guess, location_confidence, location_label, and notes. File names must be filesystem-safe, lowercase where applicable, preserve meaningful Korean/Japanese/Chinese words, and keep the original extension. Use location guesses only when metadata already supports them; do not invent places from metadata alone. " + analysisLanguageInstruction(cfg),
			},
			{
				"role":    "user",
				"content": string(userPayload),
			},
		},
		"temperature": 0.2,
	}
	content, err := callChatCompletion(ctx, cfg, requestBody)
	if err != nil {
		return []string{err.Error()}
	}

	var output llmOutput
	if err := json.Unmarshal([]byte(content), &output); err != nil {
		return []string{fmt.Sprintf("could not parse LLM JSON content: %v", err)}
	}

	byPath := map[string]*Item{}
	for index := range items {
		byPath[items[index].SourcePath] = &items[index]
	}
	var warnings []string
	for _, suggestion := range output.Items {
		item := byPath[suggestion.SourcePath]
		if item == nil {
			warnings = append(warnings, fmt.Sprintf("LLM returned unknown source_path: %s", suggestion.SourcePath))
			continue
		}
		item.Tags = mergeTagList(item.Tags, suggestion.Tags)
		if suggestion.SceneSummary != "" || suggestion.LocationGuess != "" || len(suggestion.Tags) > 0 {
			item.Content = &ContentInfo{
				SceneSummary:       strings.TrimSpace(suggestion.SceneSummary),
				LocationGuess:      strings.TrimSpace(suggestion.LocationGuess),
				LocationConfidence: round(clamp01(suggestion.LocationConfidence), 2),
				Tags:               mergeTagList(nil, suggestion.Tags),
				Model:              cfg.LLMModel,
				Notes:              strings.TrimSpace(suggestion.Notes),
			}
		}
		if suggestion.LocationLabel != "" {
			if item.Location == nil {
				item.Location = &LocationInfo{Source: "llm_metadata", Confidence: round(clamp01(suggestion.LocationConfidence), 2)}
			}
			item.Location.Label = strings.TrimSpace(suggestion.LocationLabel)
			item.Location.Notes = strings.TrimSpace(suggestion.LocationGuess)
			item.Tags = mergeTagList(item.Tags, locationTags(item.Location))
		}
		item.Tags = mergeTagList(item.Tags, contentTags(item.Content))
		if suggestion.SuggestedSlug != "" {
			item.NameParts.Slug = slugify(suggestion.SuggestedSlug)
		}
		if suggestion.FinalFileName != "" {
			finalName := sanitizeFinalFileName(suggestion.FinalFileName, item.Extension)
			if finalName == "" {
				warnings = append(warnings, fmt.Sprintf("LLM returned unsafe final filename for %s", item.SourcePath))
			} else {
				item.FinalFileName = finalName
			}
		} else if suggestion.SuggestedSlug != "" {
			item.FinalFileName = buildFileName(item.NameParts, item.Extension)
		}
		item.LLMNotes = strings.TrimSpace(suggestion.Notes)
	}
	return warnings
}

func callChatCompletion(ctx context.Context, cfg Config, requestBody map[string]any) (string, error) {
	body, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("could not encode LLM request: %w", err)
	}

	endpoint := strings.TrimRight(cfg.LLMBaseURL, "/")
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint += "/chat/completions"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("could not create LLM request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.LLMAPIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.LLMAPIKey)
	}

	resp, err := llmHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("LLM request failed: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", fmt.Errorf("could not read LLM response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("LLM request returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	var chatResponse chatCompletionResponse
	if err := json.Unmarshal(responseBody, &chatResponse); err != nil {
		return "", fmt.Errorf("could not parse LLM chat response: %w", err)
	}
	if len(chatResponse.Choices) == 0 {
		return "", fmt.Errorf("LLM response had no choices")
	}

	content := strings.TrimSpace(chatResponse.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)
	return content, nil
}

func mergeTagList(existing []string, incoming []string) []string {
	seen := map[string]bool{}
	var merged []string
	for _, tag := range append(existing, incoming...) {
		tag = slugify(tag)
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		merged = append(merged, tag)
	}
	return merged
}

func sanitizeFinalFileName(name string, expectedExt string) string {
	name = strings.ToLower(strings.TrimSpace(filepath.Base(name)))
	if name == "." || name == "/" || name == "" {
		return ""
	}
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" {
		name += expectedExt
		ext = expectedExt
	}
	if expectedExt != "" && ext != expectedExt {
		name = strings.TrimSuffix(name, ext) + expectedExt
	}
	base := strings.TrimSuffix(name, filepath.Ext(name))
	base = slugify(base)
	if base == "" {
		return ""
	}
	return base + expectedExt
}
