package media

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

//go:embed web/*
var webFiles embed.FS

type webServer struct {
	cfg            Config
	report         Report
	mu             sync.RWMutex
	shutdown       chan struct{}
	shutdownOnce   sync.Once
	clientMu       sync.Mutex
	lastClient     time.Time
	analysisMu     sync.RWMutex
	analysisStatus analysisStatus
	progress       io.Writer
}

type analysisStatus struct {
	Enabled              bool     `json:"enabled"`
	Running              bool     `json:"running"`
	Requested            int      `json:"requested"`
	Analyzed             int      `json:"analyzed"`
	Updated              int      `json:"updated"`
	SourcePaths          []string `json:"source_paths,omitempty"`
	CurrentSourcePath    string   `json:"current_source_path,omitempty"`
	CompletedSourcePaths []string `json:"completed_source_paths,omitempty"`
	FailedSourcePaths    []string `json:"failed_source_paths,omitempty"`
	Warnings             []string `json:"warnings,omitempty"`
	Error                string   `json:"error,omitempty"`
	StartedAt            string   `json:"started_at,omitempty"`
	FinishedAt           string   `json:"finished_at,omitempty"`
}

type applyRequest struct {
	Operations []applyOperation `json:"operations"`
}

type applyOperation struct {
	SourcePath   string   `json:"source_path"`
	FinalName    string   `json:"final_file_name"`
	Tags         []string `json:"tags"`
	Rename       bool     `json:"rename"`
	MoveToGroup  bool     `json:"move_to_group"`
	GroupRoot    string   `json:"group_root,omitempty"`
	TargetFolder string   `json:"target_folder,omitempty"`
	WriteSidecar bool     `json:"write_sidecar"`
	WriteXAttr   bool     `json:"write_xattr"`
}

type applyResponse struct {
	Results []applyResult `json:"results"`
	Report  Report        `json:"report"`
}

type applyResult struct {
	SourcePath   string `json:"source_path"`
	TargetPath   string `json:"target_path,omitempty"`
	Renamed      bool   `json:"renamed"`
	Moved        bool   `json:"moved"`
	Group        string `json:"group,omitempty"`
	TargetFolder string `json:"target_folder,omitempty"`
	SidecarPath  string `json:"sidecar_path,omitempty"`
	XAttrWritten bool   `json:"xattr_written"`
	Status       string `json:"status"`
	Error        string `json:"error,omitempty"`
}

type tagSidecar struct {
	Service           ServiceInfo   `json:"service"`
	UpdatedAt         string        `json:"updated_at"`
	SourcePath        string        `json:"source_path"`
	FileName          string        `json:"file_name"`
	OriginalFileName  string        `json:"original_file_name"`
	ShotAt            string        `json:"shot_at,omitempty"`
	ShotAtSource      string        `json:"shot_at_source,omitempty"`
	Tags              []string      `json:"tags"`
	Location          *LocationInfo `json:"location,omitempty"`
	Content           *ContentInfo  `json:"content,omitempty"`
	Group             *GroupInfo    `json:"group,omitempty"`
	RecommendedName   string        `json:"recommended_file_name"`
	FinalName         string        `json:"final_file_name"`
	DurationSeconds   float64       `json:"duration_seconds,omitempty"`
	FormatName        string        `json:"format_name,omitempty"`
	MetadataWriteMode string        `json:"metadata_write_mode"`
}

func runServe(args []string, stdout, stderr io.Writer, envWarnings []string) error {
	cfg := defaultConfig()
	fs := flag.NewFlagSet(cliName+" serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	addIndexFlags(fs, &cfg)
	fs.StringVar(&cfg.Host, "host", cfg.Host, "web server host")
	fs.IntVar(&cfg.Port, "port", cfg.Port, "web server port, use 0 for a random free port")
	fs.BoolVar(&cfg.AutoAnalyze, "auto-analyze", cfg.AutoAnalyze, "automatically analyze files with missing content after the web UI starts")
	fs.IntVar(&cfg.AutoAnalyzeMaxItems, "auto-analyze-max-items", cfg.AutoAnalyzeMaxItems, "maximum files to auto analyze on server start; 0 means all")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: %s serve [flags] <video-file-or-directory>...\n\n", cliName)
		fmt.Fprintln(stderr, "Launches a local file-manager web UI for reviewing names and tags.")
		fmt.Fprintln(stderr, "\nFlags:")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return errors.New("at least one file or directory is required")
	}
	if cfg.AutoAnalyzeMaxItems < 0 {
		return errors.New("--auto-analyze-max-items must be 0 or greater")
	}

	report, err := BuildReport(context.Background(), cfg, fs.Args())
	if err != nil {
		return err
	}
	report.Warnings = append(envWarnings, report.Warnings...)
	report.Summary = summarize(report.Items, report.Summary.FilesDiscovered, len(report.Warnings))

	app := &webServer{
		cfg:            cfg,
		report:         report,
		shutdown:       make(chan struct{}),
		analysisStatus: analysisStatus{Enabled: cfg.AutoAnalyze},
		progress:       stderr,
	}
	return app.serve(stdout)
}

func (s *webServer) serve(stdout io.Writer) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/favicon.svg", s.handleFavicon)
	mux.HandleFunc("/api/report", s.handleReport)
	mux.HandleFunc("/api/apply", s.handleApply)
	mux.HandleFunc("/api/analyze", s.handleAnalyze)
	mux.HandleFunc("/api/analysis-status", s.handleAnalysisStatus)
	mux.HandleFunc("/api/folders", s.handleFolders)
	mux.HandleFunc("/api/folder-plan", s.handleFolderPlan)
	mux.HandleFunc("/api/ping", s.handlePing)
	mux.HandleFunc("/api/shutdown", s.handleShutdown)
	mux.HandleFunc("/media", s.handleMedia)

	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port))
	if err != nil {
		return err
	}
	url := "http://" + listener.Addr().String()
	fmt.Fprintf(stdout, "Clip Atlas web UI: %s\n", url)
	fmt.Fprintln(stdout, "Close the web page or press Stop Server in the UI to shut this process down.")

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go s.watchClientIdle(20 * time.Second)
	if s.cfg.AutoAnalyze {
		go s.runAutoAnalyze(context.Background())
	}
	go func() {
		<-s.shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *webServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := webFiles.ReadFile("web/app.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func (s *webServer) handleFavicon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	data, err := webFiles.ReadFile("web/favicon.svg")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	_, _ = w.Write(data)
}

func (s *webServer) handleReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.RLock()
	report := s.report
	s.mu.RUnlock()
	writeJSON(w, report)
}

func (s *webServer) handleApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()

	var request applyRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, 4<<20))
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(request.Operations) == 0 {
		http.Error(w, "no operations provided", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	response := applyResponse{}
	for _, operation := range request.Operations {
		result := s.applyOne(operation)
		response.Results = append(response.Results, result)
	}
	s.report.Summary = summarize(s.report.Items, len(s.report.Items), len(s.report.Warnings))
	response.Report = s.report
	writeJSON(w, response)
}

type analyzeRequest struct {
	SourcePaths []string `json:"source_paths"`
	Frames      int      `json:"frames,omitempty"`
}

type analyzeResponse struct {
	Analyzed int      `json:"analyzed"`
	Updated  int      `json:"updated"`
	Warnings []string `json:"warnings,omitempty"`
	Report   Report   `json:"report"`
}

type folderListRequest struct {
	Root  string `json:"root"`
	Depth int    `json:"depth,omitempty"`
}

type folderListResponse struct {
	Root     string        `json:"root"`
	Folders  []folderEntry `json:"folders"`
	Warnings []string      `json:"warnings,omitempty"`
}

type folderPlanRequest struct {
	Root        string   `json:"root"`
	Depth       int      `json:"depth,omitempty"`
	SourcePaths []string `json:"source_paths"`
}

type folderPlanResponse struct {
	Root            string             `json:"root"`
	UsedLLM         bool               `json:"used_llm"`
	ExistingFolders []folderEntry      `json:"existing_folders,omitempty"`
	Folders         []plannedFolder    `json:"folders"`
	Assignments     []folderAssignment `json:"assignments"`
	Warnings        []string           `json:"warnings,omitempty"`
}

type analysisRunOptions struct {
	MaxItems int
}

func (s *webServer) handleAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()

	cfg := s.cfg
	cfg.UseLLM = true
	cfg.UseLLMVision = true
	if !supportsAudioTranscriptions(cfg.LLMBaseURL) {
		cfg.UseLLMAudio = false
	}

	var request analyzeRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(request.SourcePaths) == 0 {
		http.Error(w, "select at least one file to analyze", http.StatusBadRequest)
		return
	}
	if s.analysisIsRunning() {
		http.Error(w, "automatic analysis is already running", http.StatusConflict)
		return
	}
	if request.Frames > 0 {
		cfg.VisionFrames = request.Frames
	}
	cfg.VisionMaxItems = len(request.SourcePaths)
	cfg.AudioMaxItems = len(request.SourcePaths)
	if err := validateConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	selected := make([]Item, 0, len(request.SourcePaths))
	for _, sourcePath := range request.SourcePaths {
		index := slices.IndexFunc(s.report.Items, func(item Item) bool {
			return item.SourcePath == sourcePath
		})
		if index >= 0 {
			selected = append(selected, s.report.Items[index])
		}
	}
	s.mu.RUnlock()

	if len(selected) == 0 {
		http.Error(w, "selected files are not part of the current report", http.StatusBadRequest)
		return
	}

	sourcePaths := make([]string, 0, len(selected))
	for _, item := range selected {
		sourcePaths = append(sourcePaths, item.SourcePath)
	}
	cfg.VisionMaxItems = 1
	cfg.AudioMaxItems = 1

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(max(1, cfg.LLMTimeoutSeconds*len(sourcePaths)*2))*time.Second)
	defer cancel()
	progress := s.newAnalysisProgress("Manual analysis", len(sourcePaths))
	progress.start()
	warnings := []string{}
	updated := 0
	for index, sourcePath := range sourcePaths {
		progress.update(index, filepath.Base(sourcePath), updated, len(warnings))
		nextWarnings, nextUpdated := s.analyzeSourcePath(ctx, cfg, sourcePath, analysisRunOptions{MaxItems: len(sourcePaths)})
		warnings = append(warnings, nextWarnings...)
		updated += nextUpdated
		progress.update(index+1, filepath.Base(sourcePath), updated, len(warnings))
	}
	progress.finish(len(sourcePaths), updated, len(warnings), "")

	s.mu.RLock()
	report := s.report
	s.mu.RUnlock()

	writeJSON(w, analyzeResponse{
		Analyzed: len(sourcePaths),
		Updated:  updated,
		Warnings: warnings,
		Report:   report,
	})
}

func (s *webServer) handleFolders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()

	var request folderListRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	folders, warnings, err := listSubfolders(request.Root, request.Depth)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, folderListResponse{
		Root:     request.Root,
		Folders:  folders,
		Warnings: warnings,
	})
}

func (s *webServer) handleFolderPlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()

	var request folderPlanRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, 2<<20))
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(request.SourcePaths) == 0 {
		http.Error(w, "select at least one file to plan", http.StatusBadRequest)
		return
	}

	existingFolders, warnings, err := listSubfolders(request.Root, request.Depth)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	selected := make([]Item, 0, len(request.SourcePaths))
	for _, sourcePath := range request.SourcePaths {
		index := slices.IndexFunc(s.report.Items, func(item Item) bool {
			return item.SourcePath == sourcePath
		})
		if index >= 0 {
			selected = append(selected, s.report.Items[index])
		}
	}
	s.mu.RUnlock()
	if len(selected) == 0 {
		http.Error(w, "selected files are not part of the current report", http.StatusBadRequest)
		return
	}

	cfg := s.cfg
	cfg.UseLLM = true
	usedLLM := false
	var plan folderPlanOutput
	if strings.TrimSpace(cfg.LLMModel) != "" && strings.TrimSpace(cfg.LLMAPIKey) != "" {
		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(max(1, cfg.LLMTimeoutSeconds))*time.Second)
		defer cancel()
		llmPlan, err := planFoldersWithLLM(ctx, cfg, selected, existingFolders)
		if err != nil {
			warnings = append(warnings, "folder plan LLM failed: "+err.Error())
		} else {
			plan = completeFolderPlan(llmPlan, selected, existingFolders)
			usedLLM = true
		}
	} else {
		warnings = append(warnings, "folder plan LLM skipped: missing LLM model or API key")
	}
	if !usedLLM {
		plan = deterministicFolderPlan(selected, existingFolders)
	}

	writeJSON(w, folderPlanResponse{
		Root:            request.Root,
		UsedLLM:         usedLLM,
		ExistingFolders: existingFolders,
		Folders:         plan.Folders,
		Assignments:     plan.Assignments,
		Warnings:        warnings,
	})
}

func (s *webServer) handleAnalysisStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.analysisMu.RLock()
	status := s.analysisStatus
	status.SourcePaths = append([]string{}, s.analysisStatus.SourcePaths...)
	status.CompletedSourcePaths = append([]string{}, s.analysisStatus.CompletedSourcePaths...)
	status.FailedSourcePaths = append([]string{}, s.analysisStatus.FailedSourcePaths...)
	status.Warnings = append([]string{}, s.analysisStatus.Warnings...)
	s.analysisMu.RUnlock()
	writeJSON(w, status)
}

func (s *webServer) runAutoAnalyze(ctx context.Context) {
	paths := s.pendingAnalysisPaths()
	if s.cfg.AutoAnalyzeMaxItems > 0 && len(paths) > s.cfg.AutoAnalyzeMaxItems {
		paths = paths[:s.cfg.AutoAnalyzeMaxItems]
	}

	now := time.Now().Format(time.RFC3339)
	s.analysisMu.Lock()
	s.analysisStatus = analysisStatus{
		Enabled:     true,
		Running:     len(paths) > 0,
		Requested:   len(paths),
		SourcePaths: append([]string{}, paths...),
		StartedAt:   now,
	}
	if len(paths) == 0 {
		s.analysisStatus.FinishedAt = now
	}
	s.analysisMu.Unlock()
	if len(paths) == 0 {
		return
	}

	cfg := s.cfg
	cfg.UseLLM = true
	cfg.UseLLMVision = true
	cfg.VisionMaxItems = 1
	if cfg.UseLLMAudio {
		cfg.AudioMaxItems = 1
	}
	if err := validateConfig(cfg); err != nil {
		s.finishAnalysisStatus(err.Error())
		s.appendReportWarnings([]string{"automatic analysis skipped: " + err.Error()})
		return
	}

	progress := s.newAnalysisProgress("Auto analysis", len(paths))
	progress.start()
	warningCount := 0
	updatedCount := 0
	for index, sourcePath := range paths {
		select {
		case <-ctx.Done():
			progress.finish(index, updatedCount, warningCount, ctx.Err().Error())
			s.finishAnalysisStatus(ctx.Err().Error())
			return
		case <-s.shutdown:
			progress.finish(index, updatedCount, warningCount, "server stopped")
			s.finishAnalysisStatus("server stopped")
			return
		default:
		}

		s.markAnalysisCurrent(sourcePath)
		progress.update(index, filepath.Base(sourcePath), updatedCount, warningCount)
		warnings, updated := s.analyzeSourcePath(ctx, cfg, sourcePath, analysisRunOptions{MaxItems: s.cfg.AutoAnalyzeMaxItems})
		warningCount += len(warnings)
		updatedCount += updated
		s.analysisMu.Lock()
		s.analysisStatus.Analyzed++
		s.analysisStatus.Updated += updated
		s.analysisStatus.CurrentSourcePath = ""
		s.analysisStatus.CompletedSourcePaths = appendUniqueString(s.analysisStatus.CompletedSourcePaths, sourcePath)
		if len(warnings) > 0 {
			s.analysisStatus.FailedSourcePaths = appendUniqueString(s.analysisStatus.FailedSourcePaths, sourcePath)
		}
		s.analysisStatus.Warnings = appendLimitedWarnings(s.analysisStatus.Warnings, warnings, 20)
		s.analysisMu.Unlock()
		progress.update(index+1, filepath.Base(sourcePath), updatedCount, warningCount)
	}

	progress.finish(len(paths), updatedCount, warningCount, "")
	s.finishAnalysisStatus("")
}

func (s *webServer) pendingAnalysisPaths() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	paths := make([]string, 0, len(s.report.Items))
	for _, item := range s.report.Items {
		if item.Content != nil {
			continue
		}
		if item.Video == nil && item.Audio == nil {
			continue
		}
		paths = append(paths, item.SourcePath)
	}
	return paths
}

func (s *webServer) analyzeSourcePath(ctx context.Context, cfg Config, sourcePath string, options analysisRunOptions) ([]string, int) {
	s.mu.RLock()
	index := slices.IndexFunc(s.report.Items, func(item Item) bool {
		return item.SourcePath == sourcePath
	})
	if index == -1 {
		s.mu.RUnlock()
		return []string{"automatic analysis skipped unknown source path: " + sourcePath}, 0
	}
	selected := []Item{s.report.Items[index]}
	s.mu.RUnlock()

	itemCtx, cancel := context.WithTimeout(ctx, time.Duration(max(1, cfg.LLMTimeoutSeconds*2))*time.Second)
	defer cancel()
	warnings := []string{}
	if cfg.UseLLMVision {
		warnings = append(warnings, EnrichWithVision(itemCtx, cfg, selected)...)
	}
	if cfg.UseLLMAudio {
		warnings = append(warnings, EnrichWithAudio(itemCtx, cfg, selected)...)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	index = slices.IndexFunc(s.report.Items, func(item Item) bool {
		return item.SourcePath == sourcePath
	})
	if index == -1 {
		return warnings, 0
	}
	s.report.Items[index] = selected[0]
	s.report.Options.LLM = true
	s.report.Options.LLMVision = true
	s.report.Options.LLMAudio = cfg.UseLLMAudio
	s.report.Options.AutoAnalyze = s.cfg.AutoAnalyze
	s.report.Options.AutoAnalyzeMaxItems = s.cfg.AutoAnalyzeMaxItems
	s.report.Options.VisionFrames = cfg.VisionFrames
	s.report.Options.VisionMaxItems = options.MaxItems
	s.report.Options.AudioMaxSeconds = cfg.AudioMaxSeconds
	s.report.Options.AudioMaxItems = options.MaxItems
	s.report.Warnings = append(s.report.Warnings, warnings...)
	if selected[0].Content != nil || selected[0].Location != nil {
		if err := saveAnalysisCache(selected[0]); err != nil {
			warning := fmt.Sprintf("analysis cache write failed for %s: %v", sourcePath, err)
			warnings = append(warnings, warning)
			s.report.Warnings = append(s.report.Warnings, warning)
		}
	}
	s.report.Summary = summarize(s.report.Items, len(s.report.Items), len(s.report.Warnings))
	if selected[0].Content != nil || selected[0].Location != nil {
		return warnings, 1
	}
	return warnings, 0
}

func (s *webServer) markAnalysisCurrent(sourcePath string) {
	s.analysisMu.Lock()
	s.analysisStatus.CurrentSourcePath = sourcePath
	s.analysisMu.Unlock()
}

func (s *webServer) analysisIsRunning() bool {
	s.analysisMu.RLock()
	defer s.analysisMu.RUnlock()
	return s.analysisStatus.Running
}

func (s *webServer) finishAnalysisStatus(message string) {
	s.analysisMu.Lock()
	defer s.analysisMu.Unlock()
	s.analysisStatus.Running = false
	s.analysisStatus.FinishedAt = time.Now().Format(time.RFC3339)
	if message != "" {
		s.analysisStatus.Error = message
	}
}

func (s *webServer) appendReportWarnings(warnings []string) {
	if len(warnings) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.report.Warnings = append(s.report.Warnings, warnings...)
	s.report.Summary = summarize(s.report.Items, len(s.report.Items), len(s.report.Warnings))
}

type cliAnalysisProgress struct {
	w       io.Writer
	mu      sync.Mutex
	label   string
	total   int
	lastLen int
	started time.Time
}

func (s *webServer) newAnalysisProgress(label string, total int) *cliAnalysisProgress {
	return &cliAnalysisProgress{
		w:       s.progress,
		label:   label,
		total:   total,
		started: time.Now(),
	}
}

func (p *cliAnalysisProgress) start() {
	if p == nil || p.w == nil || p.total == 0 {
		return
	}
	p.update(0, "starting", 0, 0)
}

func (p *cliAnalysisProgress) update(done int, current string, updated int, warnings int) {
	if p == nil || p.w == nil || p.total == 0 {
		return
	}
	if done < 0 {
		done = 0
	}
	if done > p.total {
		done = p.total
	}
	width := 24
	filled := 0
	if p.total > 0 {
		filled = done * width / p.total
	}
	bar := strings.Repeat("#", filled) + strings.Repeat("-", width-filled)
	percent := done * 100 / p.total
	elapsed := time.Since(p.started).Round(time.Second)
	text := fmt.Sprintf("%s [%s] %3d%% %d/%d updated=%d warnings=%d elapsed=%s %s",
		p.label,
		bar,
		percent,
		done,
		p.total,
		updated,
		warnings,
		elapsed,
		trimCLIText(current, 34),
	)
	p.render(text)
}

func (p *cliAnalysisProgress) finish(done int, updated int, warnings int, message string) {
	if p == nil || p.w == nil || p.total == 0 {
		return
	}
	current := "done"
	if message != "" {
		current = message
	}
	p.update(done, current, updated, warnings)
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Fprintln(p.w)
	p.lastLen = 0
}

func (p *cliAnalysisProgress) render(text string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	padding := ""
	if p.lastLen > len(text) {
		padding = strings.Repeat(" ", p.lastLen-len(text))
	}
	fmt.Fprintf(p.w, "\r%s%s", text, padding)
	p.lastLen = len(text)
}

func trimCLIText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}

func appendLimitedWarnings(existing, next []string, limit int) []string {
	if limit <= 0 || len(existing) >= limit {
		return existing
	}
	remaining := limit - len(existing)
	if len(next) > remaining {
		next = next[:remaining]
	}
	return append(existing, next...)
}

func appendUniqueString(values []string, value string) []string {
	if slices.Contains(values, value) {
		return values
	}
	return append(values, value)
}

func moveCompanionFile(oldSourcePath, newSourcePath, suffix string) error {
	oldPath := oldSourcePath + suffix
	newPath := newSourcePath + suffix
	if _, err := os.Stat(oldPath); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if _, err := os.Stat(newPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		return err
	}
	return os.Rename(oldPath, newPath)
}

func (s *webServer) handlePing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.markClientSeen()
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *webServer) applyOne(operation applyOperation) applyResult {
	result := applyResult{
		SourcePath: operation.SourcePath,
		Status:     "skipped",
	}
	index := slices.IndexFunc(s.report.Items, func(item Item) bool {
		return item.SourcePath == operation.SourcePath
	})
	if index == -1 {
		result.Status = "failed"
		result.Error = "source path is not part of the current report"
		return result
	}

	item := &s.report.Items[index]
	cleanTags := mergeTagList(nil, operation.Tags)
	if len(cleanTags) == 0 {
		cleanTags = item.Tags
	}

	finalName := operation.FinalName
	if strings.TrimSpace(finalName) == "" {
		finalName = item.FinalFileName
	}
	finalName = sanitizeFinalFileName(finalName, item.Extension)
	if finalName == "" {
		result.Status = "failed"
		result.Error = "final filename is empty or unsafe"
		return result
	}

	currentPath := item.SourcePath
	candidate := *item
	candidate.Tags = append([]string{}, cleanTags...)
	candidate.FinalFileName = finalName
	updateItemGroup(&candidate)

	targetDir := filepath.Dir(currentPath)
	if operation.MoveToGroup {
		groupRoot := strings.TrimSpace(operation.GroupRoot)
		if groupRoot == "" {
			result.Status = "failed"
			result.Error = "group destination folder is required"
			return result
		}
		groupFolder := "other"
		if strings.TrimSpace(operation.TargetFolder) != "" {
			cleanedFolder, err := cleanRelativeFolder(operation.TargetFolder)
			if err != nil {
				result.Status = "failed"
				result.Error = "target folder is unsafe: " + err.Error()
				return result
			}
			groupFolder = cleanedFolder
		} else if candidate.Group != nil && candidate.Group.Folder != "" {
			groupFolder = candidate.Group.Folder
			result.Group = candidate.Group.Key
		}
		result.TargetFolder = groupFolder
		targetDir = filepath.Join(groupRoot, groupFolder)
	}

	targetPath := filepath.Join(targetDir, finalName)
	result.TargetPath = targetPath
	if (operation.Rename || operation.MoveToGroup) && targetPath != currentPath {
		if _, err := os.Stat(targetPath); err == nil {
			result.Status = "failed"
			result.Error = "target file already exists"
			return result
		} else if !errors.Is(err, os.ErrNotExist) {
			result.Status = "failed"
			result.Error = "could not check target file: " + err.Error()
			return result
		}
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			result.Status = "failed"
			result.Error = "could not create target folder: " + err.Error()
			return result
		}
		if err := os.Rename(currentPath, targetPath); err != nil {
			result.Status = "failed"
			result.Error = "move failed: " + err.Error()
			return result
		}
		_ = moveCompanionFile(currentPath, targetPath, analysisCacheSuffix)
		_ = moveCompanionFile(currentPath, targetPath, ".clip-tags.json")
		candidate.SourcePath = targetPath
		result.Renamed = filepath.Base(currentPath) != filepath.Base(targetPath)
		result.Moved = filepath.Dir(currentPath) != filepath.Dir(targetPath)
	}

	*item = candidate

	if err := saveAnalysisCache(candidate); err != nil {
		result.Status = "failed"
		result.Error = "analysis cache write failed: " + err.Error()
		return result
	}

	if operation.WriteSidecar {
		sidecarPath, err := writeTagSidecar(candidate)
		if err != nil {
			result.Status = "failed"
			result.Error = "sidecar write failed: " + err.Error()
			return result
		}
		result.SidecarPath = sidecarPath
	}

	if operation.WriteXAttr {
		if err := writeXAttr(candidate); err != nil {
			result.Status = "failed"
			result.Error = "xattr write failed: " + err.Error()
			return result
		}
		result.XAttrWritten = true
	}

	if result.Renamed || result.Moved || result.SidecarPath != "" || result.XAttrWritten {
		result.Status = "applied"
	} else {
		result.Status = "updated"
	}
	return result
}

func (s *webServer) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]string{"status": "shutting_down"})
	go func() {
		time.Sleep(150 * time.Millisecond)
		s.shutdownOnce.Do(func() {
			close(s.shutdown)
		})
	}()
}

func (s *webServer) markClientSeen() {
	s.clientMu.Lock()
	s.lastClient = time.Now()
	s.clientMu.Unlock()
}

func (s *webServer) watchClientIdle(timeout time.Duration) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.shutdown:
			return
		case <-ticker.C:
			s.clientMu.Lock()
			lastClient := s.lastClient
			s.clientMu.Unlock()
			if lastClient.IsZero() || time.Since(lastClient) < timeout {
				continue
			}
			s.shutdownOnce.Do(func() {
				close(s.shutdown)
			})
			return
		}
	}
}

func (s *webServer) handleMedia(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	allowed := slices.ContainsFunc(s.report.Items, func(item Item) bool {
		return item.SourcePath == path
	})
	s.mu.RUnlock()
	if !allowed {
		http.Error(w, "file is not part of the current report", http.StatusForbidden)
		return
	}
	http.ServeFile(w, r, path)
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(value)
}

func writeTagSidecar(item Item) (string, error) {
	sidecar := tagSidecar{
		Service:           ServiceInfo{Name: serviceName, CLI: cliName, Version: version},
		UpdatedAt:         time.Now().Format(time.RFC3339),
		SourcePath:        item.SourcePath,
		FileName:          filepath.Base(item.SourcePath),
		OriginalFileName:  item.OriginalFileName,
		ShotAt:            item.ShotAt,
		ShotAtSource:      item.ShotAtSource,
		Tags:              item.Tags,
		Location:          item.Location,
		Content:           item.Content,
		Group:             item.Group,
		RecommendedName:   item.RecommendedFileName,
		FinalName:         item.FinalFileName,
		DurationSeconds:   item.DurationSeconds,
		FormatName:        item.FormatName,
		MetadataWriteMode: "sidecar_json",
	}
	data, err := json.MarshalIndent(sidecar, "", "  ")
	if err != nil {
		return "", err
	}
	path := item.SourcePath + ".clip-tags.json"
	return path, os.WriteFile(path, append(data, '\n'), 0o644)
}

func writeXAttr(item Item) error {
	payload := map[string]any{
		"service":             serviceName,
		"updated_at":          time.Now().Format(time.RFC3339),
		"tags":                item.Tags,
		"group":               item.Group,
		"shot_at":             item.ShotAt,
		"final_file_name":     item.FinalFileName,
		"recommended_name":    item.RecommendedFileName,
		"metadata_write_mode": "macos_xattr",
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	cmd := exec.Command("xattr", "-w", "com.clipatlas.tags", string(data), item.SourcePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(output))
		if text != "" {
			return fmt.Errorf("%w: %s", err, text)
		}
		return err
	}
	return nil
}
