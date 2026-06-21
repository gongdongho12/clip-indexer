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
	cfg          Config
	report       Report
	mu           sync.RWMutex
	shutdown     chan struct{}
	shutdownOnce sync.Once
	clientMu     sync.Mutex
	lastClient   time.Time
}

type applyRequest struct {
	Operations []applyOperation `json:"operations"`
}

type applyOperation struct {
	SourcePath   string   `json:"source_path"`
	FinalName    string   `json:"final_file_name"`
	Tags         []string `json:"tags"`
	Rename       bool     `json:"rename"`
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

	report, err := BuildReport(context.Background(), cfg, fs.Args())
	if err != nil {
		return err
	}
	report.Warnings = append(envWarnings, report.Warnings...)
	report.Summary = summarize(report.Items, report.Summary.FilesDiscovered, len(report.Warnings))

	app := &webServer{
		cfg:      cfg,
		report:   report,
		shutdown: make(chan struct{}),
	}
	return app.serve(stdout)
}

func (s *webServer) serve(stdout io.Writer) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/report", s.handleReport)
	mux.HandleFunc("/api/apply", s.handleApply)
	mux.HandleFunc("/api/analyze", s.handleAnalyze)
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
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
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

func (s *webServer) handleAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()

	cfg := s.cfg
	cfg.UseLLM = true
	cfg.UseLLMVision = true
	cfg.UseLLMAudio = true

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

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(max(1, cfg.LLMTimeoutSeconds*len(selected)*2))*time.Second)
	defer cancel()
	warnings := EnrichWithVision(ctx, cfg, selected)
	warnings = append(warnings, EnrichWithAudio(ctx, cfg, selected)...)

	s.mu.Lock()
	updated := 0
	for _, analyzed := range selected {
		index := slices.IndexFunc(s.report.Items, func(item Item) bool {
			return item.SourcePath == analyzed.SourcePath
		})
		if index >= 0 {
			s.report.Items[index] = analyzed
			if analyzed.Content != nil || analyzed.Location != nil {
				updated++
			}
		}
	}
	s.report.Options.LLM = true
	s.report.Options.LLMVision = true
	s.report.Options.LLMAudio = true
	s.report.Options.VisionFrames = cfg.VisionFrames
	s.report.Options.VisionMaxItems = cfg.VisionMaxItems
	s.report.Options.AudioMaxSeconds = cfg.AudioMaxSeconds
	s.report.Options.AudioMaxItems = cfg.AudioMaxItems
	s.report.Warnings = append(s.report.Warnings, warnings...)
	s.report.Summary = summarize(s.report.Items, len(s.report.Items), len(s.report.Warnings))
	report := s.report
	s.mu.Unlock()

	writeJSON(w, analyzeResponse{
		Analyzed: len(selected),
		Updated:  updated,
		Warnings: warnings,
		Report:   report,
	})
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
	targetPath := filepath.Join(filepath.Dir(currentPath), finalName)
	result.TargetPath = targetPath

	if operation.Rename && targetPath != currentPath {
		if _, err := os.Stat(targetPath); err == nil {
			result.Status = "failed"
			result.Error = "target file already exists"
			return result
		} else if !errors.Is(err, os.ErrNotExist) {
			result.Status = "failed"
			result.Error = "could not check target file: " + err.Error()
			return result
		}
		if err := os.Rename(currentPath, targetPath); err != nil {
			result.Status = "failed"
			result.Error = "rename failed: " + err.Error()
			return result
		}
		item.SourcePath = targetPath
		result.Renamed = true
	}

	item.Tags = cleanTags
	item.FinalFileName = finalName

	if operation.WriteSidecar {
		sidecarPath, err := writeTagSidecar(*item)
		if err != nil {
			result.Status = "failed"
			result.Error = "sidecar write failed: " + err.Error()
			return result
		}
		result.SidecarPath = sidecarPath
	}

	if operation.WriteXAttr {
		if err := writeXAttr(*item); err != nil {
			result.Status = "failed"
			result.Error = "xattr write failed: " + err.Error()
			return result
		}
		result.XAttrWritten = true
	}

	if result.Renamed || result.SidecarPath != "" || result.XAttrWritten {
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
