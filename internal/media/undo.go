package media

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
)

type organizeUndo struct {
	CreatedAt string
	Items     []organizeUndoItem
}

type organizeUndoItem struct {
	Before    Item
	AfterPath string
}

type undoState struct {
	Available bool   `json:"available"`
	Count     int    `json:"count,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	Label     string `json:"label,omitempty"`
}

type undoOrganizeResponse struct {
	Undone   int                  `json:"undone"`
	Results  []undoOrganizeResult `json:"results"`
	Warnings []string             `json:"warnings,omitempty"`
	Report   Report               `json:"report"`
	Undo     undoState            `json:"undo"`
}

type undoOrganizeResult struct {
	SourcePath string `json:"source_path"`
	TargetPath string `json:"target_path,omitempty"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
}

func (s *webServer) handleUndoOrganize(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.mu.RLock()
		state := undoStateFor(s.lastOrganizeUndo)
		s.mu.RUnlock()
		writeJSON(w, state)
	case http.MethodPost:
		s.undoOrganize(w)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *webServer) undoOrganize(w http.ResponseWriter) {
	s.mu.Lock()
	if s.lastOrganizeUndo == nil || len(s.lastOrganizeUndo.Items) == 0 {
		s.mu.Unlock()
		http.Error(w, "no organize operation to undo", http.StatusConflict)
		return
	}

	undo := s.lastOrganizeUndo
	results := make([]undoOrganizeResult, 0, len(undo.Items))
	warnings := []string{}
	remaining := []organizeUndoItem{}
	undone := 0
	for index := len(undo.Items) - 1; index >= 0; index-- {
		entry := undo.Items[index]
		result := s.undoOrganizeOne(entry)
		results = append(results, result)
		if result.Status == "undone" {
			undone++
			continue
		}
		remaining = append(remaining, entry)
		if result.Error != "" {
			warnings = append(warnings, fmt.Sprintf("undo failed for %s: %s", result.SourcePath, result.Error))
		}
	}
	if len(warnings) > 0 {
		s.report.Warnings = append(s.report.Warnings, warnings...)
	}
	if len(remaining) == 0 {
		s.lastOrganizeUndo = nil
	} else {
		s.lastOrganizeUndo = &organizeUndo{
			CreatedAt: undo.CreatedAt,
			Items:     remaining,
		}
	}
	refreshReportDerived(&s.report, reportFilesDiscovered(s.report))
	report := s.report
	state := undoStateFor(s.lastOrganizeUndo)
	s.mu.Unlock()

	writeJSON(w, undoOrganizeResponse{
		Undone:   undone,
		Results:  results,
		Warnings: warnings,
		Report:   report,
		Undo:     state,
	})
}

func (s *webServer) undoOrganizeOne(entry organizeUndoItem) undoOrganizeResult {
	result := undoOrganizeResult{
		SourcePath: entry.AfterPath,
		TargetPath: entry.Before.SourcePath,
		Status:     "failed",
	}
	reportIndex := slices.IndexFunc(s.report.Items, func(item Item) bool {
		return item.SourcePath == entry.AfterPath
	})
	if reportIndex == -1 {
		result.Error = "organized file is not part of the current report"
		return result
	}
	if _, err := os.Stat(entry.AfterPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			result.Error = "organized file no longer exists"
		} else {
			result.Error = "could not check organized file: " + err.Error()
		}
		return result
	}
	if _, err := os.Stat(entry.Before.SourcePath); err == nil {
		result.Error = "original path already exists"
		return result
	} else if !errors.Is(err, os.ErrNotExist) {
		result.Error = "could not check original path: " + err.Error()
		return result
	}
	if err := os.MkdirAll(filepath.Dir(entry.Before.SourcePath), 0o755); err != nil {
		result.Error = "could not create original folder: " + err.Error()
		return result
	}
	if err := os.Rename(entry.AfterPath, entry.Before.SourcePath); err != nil {
		result.Error = "move back failed: " + err.Error()
		return result
	}
	_ = moveCompanionFile(entry.AfterPath, entry.Before.SourcePath, analysisCacheSuffix)
	_ = moveCompanionFile(entry.AfterPath, entry.Before.SourcePath, ".clip-tags.json")

	before := cloneItem(entry.Before)
	s.report.Items[reportIndex] = before
	if err := saveAnalysisCache(before); err != nil {
		s.report.Warnings = append(s.report.Warnings, fmt.Sprintf("analysis cache write failed for %s: %v", before.SourcePath, err))
	}
	result.Status = "undone"
	return result
}

func undoStateFor(undo *organizeUndo) undoState {
	if undo == nil || len(undo.Items) == 0 {
		return undoState{}
	}
	count := len(undo.Items)
	return undoState{
		Available: true,
		Count:     count,
		CreatedAt: undo.CreatedAt,
		Label:     fmt.Sprintf("Undo organize (%d)", count),
	}
}
