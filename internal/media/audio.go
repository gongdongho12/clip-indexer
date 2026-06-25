package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type audioTranscriptionResponse struct {
	Text string `json:"text"`
}

type audioItemInput struct {
	SourcePath          string        `json:"source_path"`
	OriginalFileName    string        `json:"original_file_name"`
	ShotAt              string        `json:"shot_at,omitempty"`
	DurationSeconds     float64       `json:"duration_seconds,omitempty"`
	Audio               *AudioInfo    `json:"audio,omitempty"`
	Location            *LocationInfo `json:"location,omitempty"`
	Content             *ContentInfo  `json:"content,omitempty"`
	Tags                []string      `json:"tags"`
	RecommendedFileName string        `json:"recommended_file_name"`
	Transcript          string        `json:"transcript"`
}

type audioOutput struct {
	Items []audioItemOutput `json:"items"`
}

type audioItemOutput struct {
	SourcePath         string   `json:"source_path"`
	Tags               []string `json:"tags,omitempty"`
	AudioSummary       string   `json:"audio_summary,omitempty"`
	LocationGuess      string   `json:"location_guess,omitempty"`
	LocationConfidence float64  `json:"location_confidence,omitempty"`
	LocationLabel      string   `json:"location_label,omitempty"`
	SuggestedSlug      string   `json:"suggested_slug,omitempty"`
	FinalFileName      string   `json:"final_file_name,omitempty"`
	Notes              string   `json:"notes,omitempty"`
}

func EnrichWithAudio(ctx context.Context, cfg Config, items []Item) []string {
	var warnings []string
	limit := len(items)
	if cfg.AudioMaxItems > 0 && cfg.AudioMaxItems < limit {
		limit = cfg.AudioMaxItems
		warnings = append(warnings, fmt.Sprintf("audio analysis limited to first %d of %d items", limit, len(items)))
	}

	analyzed := 0
	for index := range items {
		if analyzed >= limit {
			break
		}
		if items[index].Audio == nil {
			continue
		}
		output, transcript, seconds, itemWarnings := analyzeItemWithAudio(ctx, cfg, items[index])
		warnings = append(warnings, itemWarnings...)
		if transcript != "" {
			ensureContent(&items[index])
			items[index].Content.AudioTranscript = transcript
			items[index].Content.AudioSeconds = seconds
			items[index].Content.AudioModel = cfg.AudioModel
			items[index].Tags = mergeTagList(items[index].Tags, []string{"speech"})
		}
		if output != nil {
			applyAudioOutput(&items[index], *output, transcript, seconds, cfg)
		}
		analyzed++
	}
	return warnings
}

func analyzeItemWithAudio(ctx context.Context, cfg Config, item Item) (*audioItemOutput, string, int, []string) {
	audioPath, seconds, cleanup, err := extractAudioSample(ctx, cfg, item)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return nil, "", 0, []string{fmt.Sprintf("audio extraction failed for %s: %v", item.SourcePath, err)}
	}

	transcript, err := callAudioTranscription(ctx, cfg, audioPath)
	if err != nil {
		return nil, "", seconds, []string{fmt.Sprintf("audio transcription failed for %s: %v", item.SourcePath, err)}
	}
	transcript = strings.TrimSpace(transcript)
	if transcript == "" {
		return nil, "", seconds, []string{fmt.Sprintf("audio transcript was empty for %s", item.SourcePath)}
	}

	output, warnings := analyzeTranscriptWithLLM(ctx, cfg, item, transcript)
	return output, transcript, seconds, warnings
}

func analyzeTranscriptWithLLM(ctx context.Context, cfg Config, item Item, transcript string) (*audioItemOutput, []string) {
	input := audioItemInput{
		SourcePath:          item.SourcePath,
		OriginalFileName:    item.OriginalFileName,
		ShotAt:              item.ShotAt,
		DurationSeconds:     item.DurationSeconds,
		Audio:               item.Audio,
		Location:            item.Location,
		Content:             item.Content,
		Tags:                item.Tags,
		RecommendedFileName: item.RecommendedFileName,
		Transcript:          transcript,
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return nil, []string{fmt.Sprintf("could not encode audio metadata for %s: %v", item.SourcePath, err)}
	}

	requestBody := map[string]any{
		"model": cfg.LLMModel,
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": "You organize travel video clips from audio transcripts. Return only JSON: {\"items\":[{\"source_path\":\"...\",\"tags\":[\"...\"],\"audio_summary\":\"...\",\"location_guess\":\"...\",\"location_confidence\":0.0,\"location_label\":\"...\",\"suggested_slug\":\"...\",\"final_file_name\":\"...\",\"notes\":\"...\"}]}. Extract concise tags from spoken words, announcements, business/place names, transit terms, foods, activities, languages, and travel context. If the transcript strongly suggests a known place, include location_label and also include the place name in tags. Be cautious and do not invent exact coordinates.",
			},
			{
				"role":    "user",
				"content": string(payload),
			},
		},
		"temperature": 0.1,
	}

	content, err := callChatCompletion(ctx, cfg, requestBody)
	if err != nil {
		return nil, []string{fmt.Sprintf("audio LLM failed for %s: %v", item.SourcePath, err)}
	}

	var output audioOutput
	if err := json.Unmarshal([]byte(content), &output); err != nil {
		return nil, []string{fmt.Sprintf("could not parse audio JSON for %s: %v", item.SourcePath, err)}
	}
	if len(output.Items) == 0 {
		return nil, []string{fmt.Sprintf("audio response had no items for %s", item.SourcePath)}
	}
	for _, suggestion := range output.Items {
		if suggestion.SourcePath == item.SourcePath {
			return &suggestion, nil
		}
	}
	return &output.Items[0], []string{fmt.Sprintf("audio response did not echo source_path for %s", item.SourcePath)}
}

func applyAudioOutput(item *Item, suggestion audioItemOutput, transcript string, seconds int, cfg Config) {
	ensureContent(item)
	item.Content.AudioTranscript = transcript
	item.Content.AudioSummary = strings.TrimSpace(suggestion.AudioSummary)
	item.Content.AudioSeconds = seconds
	item.Content.AudioModel = cfg.AudioModel
	item.Content.AudioTags = mergeTagList(nil, suggestion.Tags)
	if item.Content.LocationGuess == "" && suggestion.LocationGuess != "" {
		item.Content.LocationGuess = strings.TrimSpace(suggestion.LocationGuess)
		item.Content.LocationConfidence = round(clamp01(suggestion.LocationConfidence), 2)
	}
	item.Content.Notes = strings.TrimSpace(strings.Join(nonEmpty(item.Content.Notes, suggestion.Notes), "\n"))

	derivedTags := append([]string{}, suggestion.Tags...)
	if transcript != "" {
		derivedTags = append(derivedTags, "speech")
	}
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
				Source:     "llm_audio",
				Confidence: round(clamp01(suggestion.LocationConfidence), 2),
			}
		}
		item.Location.Label = strings.TrimSpace(suggestion.LocationLabel)
		item.Location.Notes = strings.TrimSpace(suggestion.LocationGuess)
		item.Tags = mergeTagList(item.Tags, locationTags(item.Location))
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
	if item.Content.AudioSummary != "" {
		item.LLMNotes = strings.TrimSpace(strings.Join(nonEmpty(item.LLMNotes, item.Content.AudioSummary), "\n"))
	}
}

func extractAudioSample(ctx context.Context, cfg Config, item Item) (string, int, func(), error) {
	tempDir, err := os.MkdirTemp("", "clip-indexer-audio-*")
	if err != nil {
		return "", 0, nil, err
	}
	cleanup := func() {
		_ = os.RemoveAll(tempDir)
	}

	seconds := cfg.AudioMaxSeconds
	if seconds < 1 {
		seconds = 1
	}
	if item.DurationSeconds > 0 && item.DurationSeconds < float64(seconds) {
		seconds = max(1, int(item.DurationSeconds))
	}

	audioPath := filepath.Join(tempDir, "audio.wav")
	cmd := exec.CommandContext(
		ctx,
		cfg.FFMpegPath,
		"-v", "error",
		"-y",
		"-i", item.SourcePath,
		"-vn",
		"-t", fmt.Sprintf("%d", seconds),
		"-ac", "1",
		"-ar", "16000",
		"-c:a", "pcm_s16le",
		audioPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		cleanup()
		return "", 0, nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return audioPath, seconds, cleanup, nil
}

func callAudioTranscription(ctx context.Context, cfg Config, audioPath string) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("model", cfg.AudioModel); err != nil {
		return "", err
	}
	if err := writer.WriteField("response_format", "json"); err != nil {
		return "", err
	}
	fileWriter, err := writer.CreateFormFile("file", filepath.Base(audioPath))
	if err != nil {
		return "", err
	}
	file, err := os.Open(audioPath)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(fileWriter, file); err != nil {
		_ = file.Close()
		return "", err
	}
	_ = file.Close()
	if err := writer.Close(); err != nil {
		return "", err
	}

	endpoint := strings.TrimRight(cfg.LLMBaseURL, "/")
	if !strings.HasSuffix(endpoint, "/audio/transcriptions") {
		endpoint += "/audio/transcriptions"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if cfg.LLMAPIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.LLMAPIKey)
	}

	resp, err := llmHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("audio transcription returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	var parsed audioTranscriptionResponse
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return strings.TrimSpace(string(responseBody)), nil
	}
	return strings.TrimSpace(parsed.Text), nil
}

func ensureContent(item *Item) {
	if item.Content == nil {
		item.Content = &ContentInfo{}
	}
}
