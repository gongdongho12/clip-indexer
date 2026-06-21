package media

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type visionItemInput struct {
	SourcePath          string        `json:"source_path"`
	OriginalFileName    string        `json:"original_file_name"`
	ShotAt              string        `json:"shot_at,omitempty"`
	DurationSeconds     float64       `json:"duration_seconds,omitempty"`
	Video               *VideoInfo    `json:"video,omitempty"`
	Audio               *AudioInfo    `json:"audio,omitempty"`
	Location            *LocationInfo `json:"location,omitempty"`
	Tags                []string      `json:"tags"`
	RecommendedFileName string        `json:"recommended_file_name"`
}

type visionOutput struct {
	Items []visionItemOutput `json:"items"`
}

type visionItemOutput struct {
	SourcePath         string   `json:"source_path"`
	Tags               []string `json:"tags,omitempty"`
	SceneSummary       string   `json:"scene_summary,omitempty"`
	LocationGuess      string   `json:"location_guess,omitempty"`
	LocationConfidence float64  `json:"location_confidence,omitempty"`
	LocationLabel      string   `json:"location_label,omitempty"`
	SuggestedSlug      string   `json:"suggested_slug,omitempty"`
	FinalFileName      string   `json:"final_file_name,omitempty"`
	Notes              string   `json:"notes,omitempty"`
}

func EnrichWithVision(ctx context.Context, cfg Config, items []Item) []string {
	var warnings []string
	limit := len(items)
	if cfg.VisionMaxItems > 0 && cfg.VisionMaxItems < limit {
		limit = cfg.VisionMaxItems
		warnings = append(warnings, fmt.Sprintf("vision analysis limited to first %d of %d items", limit, len(items)))
	}

	analyzed := 0
	for index := range items {
		if analyzed >= limit {
			break
		}
		if items[index].Video == nil {
			continue
		}
		output, frameCount, itemWarnings := analyzeItemWithVision(ctx, cfg, items[index])
		warnings = append(warnings, itemWarnings...)
		if output == nil {
			analyzed++
			continue
		}
		applyVisionOutput(&items[index], *output, frameCount, cfg.LLMModel)
		analyzed++
	}
	return warnings
}

func analyzeItemWithVision(ctx context.Context, cfg Config, item Item) (*visionItemOutput, int, []string) {
	frames, cleanup, err := extractVisionFrames(ctx, cfg, item)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return nil, 0, []string{fmt.Sprintf("vision frame extraction failed for %s: %v", item.SourcePath, err)}
	}
	if len(frames) == 0 {
		return nil, 0, []string{fmt.Sprintf("no vision frames extracted for %s", item.SourcePath)}
	}

	input := visionItemInput{
		SourcePath:          item.SourcePath,
		OriginalFileName:    item.OriginalFileName,
		ShotAt:              item.ShotAt,
		DurationSeconds:     item.DurationSeconds,
		Video:               item.Video,
		Audio:               item.Audio,
		Location:            item.Location,
		Tags:                item.Tags,
		RecommendedFileName: item.RecommendedFileName,
	}
	metadata, err := json.Marshal(input)
	if err != nil {
		return nil, len(frames), []string{fmt.Sprintf("could not encode vision metadata for %s: %v", item.SourcePath, err)}
	}

	userContent := []map[string]any{
		{
			"type": "text",
			"text": "Analyze these sampled frames and metadata. Return JSON only.\n\n" + string(metadata),
		},
	}
	for _, frame := range frames {
		userContent = append(userContent, map[string]any{
			"type": "image_url",
			"image_url": map[string]any{
				"url":    "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(frame),
				"detail": "low",
			},
		})
	}

	requestBody := map[string]any{
		"model": cfg.LLMModel,
		"messages": []map[string]any{
			{
				"role":    "system",
				"content": "You analyze travel video frames. Infer visible scene content, useful concise tags, and a cautious location guess if landmarks, signs, transit names, coastline, mountains, buildings, or other visual clues support it. Include scene/activity tags such as street, restaurant, hotel, airport, train, beach, mountain, night_view, walking, driving, food, people, indoor, outdoor. If a place is visually identifiable, include location_label and also include that place name in tags. Do not invent precise coordinates. Return only JSON: {\"items\":[{\"source_path\":\"...\",\"tags\":[\"...\"],\"scene_summary\":\"...\",\"location_guess\":\"...\",\"location_confidence\":0.0,\"location_label\":\"...\",\"suggested_slug\":\"...\",\"final_file_name\":\"...\",\"notes\":\"...\"}]}",
			},
			{
				"role":    "user",
				"content": userContent,
			},
		},
		"temperature": 0.1,
	}

	content, err := callChatCompletion(ctx, cfg, requestBody)
	if err != nil {
		return nil, len(frames), []string{fmt.Sprintf("vision LLM failed for %s: %v", item.SourcePath, err)}
	}

	var output visionOutput
	if err := json.Unmarshal([]byte(content), &output); err != nil {
		return nil, len(frames), []string{fmt.Sprintf("could not parse vision JSON for %s: %v", item.SourcePath, err)}
	}
	if len(output.Items) == 0 {
		return nil, len(frames), []string{fmt.Sprintf("vision response had no items for %s", item.SourcePath)}
	}
	for _, suggestion := range output.Items {
		if suggestion.SourcePath == item.SourcePath {
			return &suggestion, len(frames), nil
		}
	}
	return &output.Items[0], len(frames), []string{fmt.Sprintf("vision response did not echo source_path for %s", item.SourcePath)}
}

func applyVisionOutput(item *Item, suggestion visionItemOutput, frameCount int, model string) {
	derivedTags := append([]string{}, suggestion.Tags...)
	if suggestion.LocationLabel != "" {
		derivedTags = append(derivedTags, suggestion.LocationLabel)
	}
	if suggestion.LocationGuess != "" && suggestion.LocationConfidence >= 0.45 {
		derivedTags = append(derivedTags, suggestion.LocationGuess)
	}
	item.Tags = mergeTagList(item.Tags, derivedTags)
	if suggestion.LocationLabel != "" {
		if item.Location == nil {
			item.Location = &LocationInfo{
				Source:     "llm_vision",
				Confidence: round(clamp01(suggestion.LocationConfidence), 2),
			}
		}
		item.Location.Label = strings.TrimSpace(suggestion.LocationLabel)
		item.Location.Notes = strings.TrimSpace(suggestion.LocationGuess)
		item.Tags = mergeTagList(item.Tags, locationTags(item.Location))
	}
	item.Content = &ContentInfo{
		SceneSummary:       strings.TrimSpace(suggestion.SceneSummary),
		LocationGuess:      strings.TrimSpace(suggestion.LocationGuess),
		LocationConfidence: round(clamp01(suggestion.LocationConfidence), 2),
		Tags:               mergeTagList(nil, suggestion.Tags),
		FrameCount:         frameCount,
		Model:              model,
		Notes:              strings.TrimSpace(suggestion.Notes),
	}
	item.Tags = mergeTagList(item.Tags, contentTags(item.Content))
	if suggestion.SuggestedSlug != "" {
		item.NameParts.Slug = slugify(suggestion.SuggestedSlug)
		item.FinalFileName = buildFileName(item.NameParts, item.Extension)
	}
	if suggestion.FinalFileName != "" {
		if finalName := sanitizeFinalFileName(suggestion.FinalFileName, item.Extension); finalName != "" {
			item.FinalFileName = finalName
		}
	}
	if item.Content.SceneSummary != "" {
		item.LLMNotes = strings.TrimSpace(strings.Join(nonEmpty(item.LLMNotes, item.Content.SceneSummary), "\n"))
	}
}

func extractVisionFrames(ctx context.Context, cfg Config, item Item) ([][]byte, func(), error) {
	tempDir, err := os.MkdirTemp("", "clip-indexer-frames-*")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		_ = os.RemoveAll(tempDir)
	}

	count := max(1, cfg.VisionFrames)
	var frames [][]byte
	for index, second := range sampleSeconds(item.DurationSeconds, count) {
		framePath := filepath.Join(tempDir, fmt.Sprintf("frame_%02d.jpg", index+1))
		cmd := exec.CommandContext(
			ctx,
			cfg.FFMpegPath,
			"-v", "error",
			"-ss", fmt.Sprintf("%.3f", second),
			"-i", item.SourcePath,
			"-map", "0:v:0",
			"-frames:v", "1",
			"-vf", "scale=960:-2",
			"-q:v", "4",
			framePath,
		)
		output, err := cmd.CombinedOutput()
		if err != nil {
			if len(frames) > 0 {
				continue
			}
			cleanup()
			return nil, nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
		}
		data, err := os.ReadFile(framePath)
		if err != nil {
			if len(frames) > 0 {
				continue
			}
			cleanup()
			return nil, nil, err
		}
		frames = append(frames, data)
	}
	return frames, cleanup, nil
}

func sampleSeconds(duration float64, count int) []float64 {
	if count < 1 {
		count = 1
	}
	if duration <= 0 {
		duration = 1
	}
	var seconds []float64
	for i := 1; i <= count; i++ {
		second := duration * float64(i) / float64(count+1)
		if second < 0.1 {
			second = 0.1
		}
		seconds = append(seconds, second)
	}
	return seconds
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func nonEmpty(values ...string) []string {
	var result []string
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, strings.TrimSpace(value))
		}
	}
	return result
}
