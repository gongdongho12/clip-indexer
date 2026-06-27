package media

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

func TestHandleOrganizeWritesRootMapAndMovesFiles(t *testing.T) {
	dir := t.TempDir()
	sourceDir := filepath.Join(dir, "input")
	groupRoot := filepath.Join(dir, "organized")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(sourceDir, "clip.mp4")
	if err := os.WriteFile(source, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := &webServer{
		report: Report{
			Items: []Item{{
				SourcePath:       source,
				OriginalFileName: filepath.Base(source),
				Extension:        ".mp4",
				Tags:             []string{"food", "cafe"},
				FinalFileName:    "clip.mp4",
			}},
		},
	}

	body, err := json.Marshal(organizeRequest{
		Root:        groupRoot,
		SourcePaths: []string{source},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/organize", bytes.NewReader(body))
	response := httptest.NewRecorder()

	server.handleOrganize(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", response.Code, response.Body.String())
	}
	var payload organizeResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	target := filepath.Join(groupRoot, "food", "clip.mp4")
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("expected organized file: %v", err)
	}
	if payload.MapPath != filepath.Join(groupRoot, "clip-atlas-map.json") {
		t.Fatalf("expected map path under root, got %q", payload.MapPath)
	}
	if _, err := os.Stat(payload.MapPath); err != nil {
		t.Fatalf("expected map file: %v", err)
	}
	mapData, err := os.ReadFile(payload.MapPath)
	if err != nil {
		t.Fatalf("read map file: %v", err)
	}
	var orgMap organizationMap
	if err := json.Unmarshal(mapData, &orgMap); err != nil {
		t.Fatalf("decode map file: %v", err)
	}
	if len(orgMap.Items) != 1 || orgMap.Items[0].SourcePath != source || orgMap.Items[0].TargetPath != target {
		t.Fatalf("expected source and target paths in map, got %#v", orgMap.Items)
	}
	if server.report.Items[0].SourcePath != target {
		t.Fatalf("expected report path to update, got %s", server.report.Items[0].SourcePath)
	}
	if len(payload.Results) != 1 || payload.Results[0].Status != "applied" {
		t.Fatalf("expected applied result, got %#v", payload.Results)
	}
}

func TestHandleOrganizeUsesDefaultRootWhenMissing(t *testing.T) {
	dir := t.TempDir()
	sourceDir := filepath.Join(dir, "input")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(sourceDir, "clip.mp4")
	if err := os.WriteFile(source, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := &webServer{
		report: Report{
			Items: []Item{{
				SourcePath:       source,
				OriginalFileName: filepath.Base(source),
				Extension:        ".mp4",
				Tags:             []string{"food", "cafe"},
				FinalFileName:    "clip.mp4",
			}},
		},
	}

	body, err := json.Marshal(organizeRequest{SourcePaths: []string{source}})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/organize", bytes.NewReader(body))
	response := httptest.NewRecorder()

	server.handleOrganize(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", response.Code, response.Body.String())
	}
	var payload organizeResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	defaultRoot := filepath.Join(sourceDir, "clip-atlas-organized")
	target := filepath.Join(defaultRoot, "food", "clip.mp4")
	if payload.Root != defaultRoot {
		t.Fatalf("expected default root %q, got %q", defaultRoot, payload.Root)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("expected organized file under default root: %v", err)
	}
}

func TestHandleUndoOrganizeMovesFilesBack(t *testing.T) {
	dir := t.TempDir()
	sourceDir := filepath.Join(dir, "input")
	groupRoot := filepath.Join(dir, "organized")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(sourceDir, "clip.mp4")
	if err := os.WriteFile(source, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := &webServer{
		report: Report{
			Items: []Item{{
				SourcePath:       source,
				MediaType:        mediaTypeVideo,
				OriginalFileName: filepath.Base(source),
				Extension:        ".mp4",
				Tags:             []string{"food", "cafe"},
				FinalFileName:    "clip.mp4",
			}},
		},
	}

	body, err := json.Marshal(organizeRequest{Root: groupRoot, SourcePaths: []string{source}})
	if err != nil {
		t.Fatal(err)
	}
	organizeRequest := httptest.NewRequest(http.MethodPost, "/api/organize", bytes.NewReader(body))
	organizeResponseRecorder := httptest.NewRecorder()
	server.handleOrganize(organizeResponseRecorder, organizeRequest)
	if organizeResponseRecorder.Code != http.StatusOK {
		t.Fatalf("expected organize ok, got %d: %s", organizeResponseRecorder.Code, organizeResponseRecorder.Body.String())
	}
	target := filepath.Join(groupRoot, "food", "clip.mp4")
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("expected organized file: %v", err)
	}
	var organizePayload organizeResponse
	if err := json.Unmarshal(organizeResponseRecorder.Body.Bytes(), &organizePayload); err != nil {
		t.Fatal(err)
	}
	if !organizePayload.Undo.Available || organizePayload.Undo.Count != 1 {
		t.Fatalf("expected undo state after organize, got %#v", organizePayload.Undo)
	}

	undoRequest := httptest.NewRequest(http.MethodPost, "/api/undo-organize", nil)
	undoResponseRecorder := httptest.NewRecorder()
	server.handleUndoOrganize(undoResponseRecorder, undoRequest)
	if undoResponseRecorder.Code != http.StatusOK {
		t.Fatalf("expected undo ok, got %d: %s", undoResponseRecorder.Code, undoResponseRecorder.Body.String())
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("expected original file after undo: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("expected organized target removed, stat err=%v", err)
	}
	var undoPayload undoOrganizeResponse
	if err := json.Unmarshal(undoResponseRecorder.Body.Bytes(), &undoPayload); err != nil {
		t.Fatal(err)
	}
	if undoPayload.Undone != 1 || undoPayload.Undo.Available {
		t.Fatalf("expected undo to be consumed, got %#v", undoPayload)
	}
	if server.report.Items[0].SourcePath != source {
		t.Fatalf("expected report source path to be restored, got %s", server.report.Items[0].SourcePath)
	}
}

func TestHandleFolderPlanUsesDefaultRootWhenMissing(t *testing.T) {
	dir := t.TempDir()
	sourceDir := filepath.Join(dir, "input")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(sourceDir, "clip.mp4")
	if err := os.WriteFile(source, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := &webServer{
		report: Report{
			Items: []Item{{
				SourcePath:       source,
				OriginalFileName: filepath.Base(source),
				Extension:        ".mp4",
				Tags:             []string{"food", "cafe"},
				FinalFileName:    "clip.mp4",
			}},
		},
	}

	body, err := json.Marshal(folderPlanRequest{SourcePaths: []string{source}})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/folder-plan", bytes.NewReader(body))
	response := httptest.NewRecorder()

	server.handleFolderPlan(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", response.Code, response.Body.String())
	}
	var payload folderPlanResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	defaultRoot := filepath.Join(sourceDir, "clip-atlas-organized")
	if payload.Root != defaultRoot {
		t.Fatalf("expected default root %q, got %q", defaultRoot, payload.Root)
	}
	if len(payload.Assignments) != 1 || payload.Assignments[0].SourcePath != source {
		t.Fatalf("expected folder assignment for source, got %#v", payload.Assignments)
	}
	if info, err := os.Stat(defaultRoot); err != nil || !info.IsDir() {
		t.Fatalf("expected default root directory to be created, info=%#v err=%v", info, err)
	}
}

func TestHandleOrganizeDoesNotAnalyzePendingFiles(t *testing.T) {
	dir := t.TempDir()
	sourceDir := filepath.Join(dir, "input")
	groupRoot := filepath.Join(dir, "organized")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(sourceDir, "clip.mp4")
	if err := os.WriteFile(source, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := &webServer{
		report: Report{
			Items: []Item{{
				SourcePath:       source,
				OriginalFileName: filepath.Base(source),
				Extension:        ".mp4",
				Video:            &VideoInfo{Codec: "h264"},
				Tags:             []string{"city"},
				FinalFileName:    "clip.mp4",
			}},
		},
	}

	body, err := json.Marshal(organizeRequest{
		Root:        groupRoot,
		SourcePaths: []string{source},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/organize", bytes.NewReader(body))
	response := httptest.NewRecorder()

	server.handleOrganize(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", response.Code, response.Body.String())
	}
	var payload organizeResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Analyzed != 0 || payload.Updated != 0 {
		t.Fatalf("organize should not run analysis, got analyzed=%d updated=%d", payload.Analyzed, payload.Updated)
	}
	for _, warning := range payload.Warnings {
		if strings.Contains(warning, "organization analysis skipped") {
			t.Fatalf("organize should not emit analysis warnings, got %q", warning)
		}
	}
}

func TestHandleOrganizeRequiresSelectedFiles(t *testing.T) {
	dir := t.TempDir()
	server := &webServer{
		report: Report{
			Items: []Item{{
				SourcePath:    filepath.Join(dir, "clip.mp4"),
				Extension:     ".mp4",
				Tags:          []string{"food"},
				FinalFileName: "clip.mp4",
			}},
		},
	}

	body, err := json.Marshal(organizeRequest{Root: filepath.Join(dir, "organized")})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/organize", bytes.NewReader(body))
	response := httptest.NewRecorder()

	server.handleOrganize(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request, got %d: %s", response.Code, response.Body.String())
	}
}

func TestHandleClearAnalysisClearsSelectedItemsAndCaches(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(source, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(analysisCachePath(source), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := &webServer{
		report: Report{
			Items: []Item{{
				SourcePath:          source,
				OriginalFileName:    filepath.Base(source),
				Extension:           ".mp4",
				Tags:                []string{"video", "street", "speech", "seoul_station"},
				NameParts:           NameParts{Date: "20240615", Time: "120000", Slug: "seoul_station", Sequence: "001"},
				RecommendedFileName: "seoul_station_scene.mp4",
				FinalFileName:       "seoul_station_scene.mp4",
				Location: &LocationInfo{
					Label:      "Seoul Station",
					Source:     "llm_vision",
					Confidence: 0.8,
				},
				Content: &ContentInfo{
					SceneSummary:       "People walking near Seoul Station.",
					AudioTranscript:    "Next stop is Seoul Station.",
					LocationGuess:      "Seoul Station",
					LocationConfidence: 0.8,
					Tags:               []string{"street"},
				},
				LLMNotes: "People walking near Seoul Station.",
			}},
		},
	}

	body, err := json.Marshal(clearAnalysisRequest{SourcePaths: []string{source}})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/clear-analysis", bytes.NewReader(body))
	response := httptest.NewRecorder()

	server.handleClearAnalysis(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", response.Code, response.Body.String())
	}
	var payload clearAnalysisResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Cleared != 1 || payload.RemovedCaches != 1 {
		t.Fatalf("expected one cleared item and cache, got %#v", payload)
	}
	if _, err := os.Stat(analysisCachePath(source)); !os.IsNotExist(err) {
		t.Fatalf("expected analysis cache to be removed, stat err=%v", err)
	}
	item := server.report.Items[0]
	if item.Content != nil || item.Location != nil || item.LLMNotes != "" {
		t.Fatalf("expected LLM analysis fields to be cleared, got %#v", item)
	}
	if slices.Contains(item.Tags, "street") || slices.Contains(item.Tags, "speech") || slices.Contains(item.Tags, "seoul_station") {
		t.Fatalf("expected content-derived tags removed, got %v", item.Tags)
	}
	if !slices.Contains(item.Tags, "video") {
		t.Fatalf("expected base tags to remain, got %v", item.Tags)
	}
	if item.FinalFileName == "seoul_station_scene.mp4" || item.RecommendedFileName != item.FinalFileName {
		t.Fatalf("expected filename suggestion to reset, got recommended=%q final=%q", item.RecommendedFileName, item.FinalFileName)
	}
	if payload.Report.Summary.WithContent != 0 {
		t.Fatalf("expected summary content count to reset, got %#v", payload.Report.Summary)
	}
}

func TestClearItemAnalysisPreservesMetadataLocation(t *testing.T) {
	item := Item{
		Extension: ".mp4",
		Tags:      []string{"video", "gps", "geo_37_5665_126_9780", "cafe"},
		NameParts: NameParts{Date: "20240615", Time: "120000", Slug: "cafe", Sequence: "001"},
		Location: &LocationInfo{
			Latitude:   37.5665,
			Longitude:  126.9780,
			Label:      "Seoul City Hall",
			Source:     "gpslatitude,gpslongitude",
			Confidence: 0.9,
		},
		Content: &ContentInfo{
			SceneSummary: "Cafe table.",
			Tags:         []string{"cafe"},
		},
		LLMNotes: "Cafe table.",
	}

	if !clearItemAnalysis(&item) {
		t.Fatalf("expected item to change")
	}
	if item.Content != nil || item.LLMNotes != "" {
		t.Fatalf("expected content fields to be cleared, got %#v", item)
	}
	if item.Location == nil || item.Location.Source != "gpslatitude,gpslongitude" {
		t.Fatalf("expected metadata location to remain, got %#v", item.Location)
	}
	if slices.Contains(item.Tags, "cafe") || !slices.Contains(item.Tags, "gps") {
		t.Fatalf("expected content tags removed and gps tags preserved, got %v", item.Tags)
	}
}

func TestManualAnalysisStatusTracksCurrentAndCompletion(t *testing.T) {
	server := &webServer{}
	sourcePaths := []string{"/tmp/a.mp4", "/tmp/b.mp4"}

	server.startAnalysisStatus(sourcePaths)
	server.markAnalysisCurrent(sourcePaths[0])

	request := httptest.NewRequest(http.MethodGet, "/api/analysis-status", nil)
	response := httptest.NewRecorder()
	server.handleAnalysisStatus(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", response.Code, response.Body.String())
	}
	var status analysisStatus
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if !status.Running || status.CurrentSourcePath != sourcePaths[0] {
		t.Fatalf("expected current analysis status, got %#v", status)
	}

	server.recordAnalysisResult(sourcePaths[0], 1, nil)
	server.finishAnalysisStatus("")

	response = httptest.NewRecorder()
	server.handleAnalysisStatus(response, request)
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode finished status: %v", err)
	}
	if status.Running || status.Analyzed != 1 || status.Updated != 1 {
		t.Fatalf("expected finished analysis counts, got %#v", status)
	}
	if !slices.Contains(status.CompletedSourcePaths, sourcePaths[0]) {
		t.Fatalf("expected completed source path, got %#v", status.CompletedSourcePaths)
	}
}
