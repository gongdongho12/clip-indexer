package media

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

//go:embed apple_vision.swift
var appleVisionSource string

var appleVisionCompileMu sync.Mutex

type localVisionLabel struct {
	Name       string  `json:"name"`
	Confidence float64 `json:"confidence"`
}

type localVisionObservation struct {
	Frame          int                `json:"frame"`
	Second         float64            `json:"second"`
	Labels         []localVisionLabel `json:"labels,omitempty"`
	RecognizedText []string           `json:"recognized_text,omitempty"`
	Error          string             `json:"error,omitempty"`
}

type appleVisionResult struct {
	Labels         []localVisionLabel `json:"labels"`
	RecognizedText []string           `json:"recognized_text"`
	Error          string             `json:"error"`
}

func describeFramesWithAppleVision(ctx context.Context, frames []visionFrame) ([]localVisionObservation, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("local vision mode requires macOS with Apple Vision")
	}
	if len(frames) == 0 {
		return nil, errors.New("local vision mode received no frames")
	}

	executable, err := appleVisionExecutable(ctx)
	if err != nil {
		return nil, err
	}
	paths, cleanup, err := localVisionFramePaths(frames)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return nil, err
	}

	output, err := exec.CommandContext(ctx, executable, paths...).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("Apple Vision helper failed: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("Apple Vision helper failed: %w", err)
	}

	var results []appleVisionResult
	if err := json.Unmarshal(output, &results); err != nil {
		return nil, fmt.Errorf("could not parse Apple Vision output: %w", err)
	}
	if len(results) != len(frames) {
		return nil, fmt.Errorf("Apple Vision returned %d observations for %d frames", len(results), len(frames))
	}

	observations := make([]localVisionObservation, 0, len(results))
	successes := 0
	for index, result := range results {
		if result.Error == "" {
			successes++
		}
		observations = append(observations, localVisionObservation{
			Frame:          index + 1,
			Second:         frames[index].Second,
			Labels:         result.Labels,
			RecognizedText: result.RecognizedText,
			Error:          result.Error,
		})
	}
	if successes == 0 {
		return nil, errors.New("Apple Vision could not analyze any extracted frame")
	}
	return observations, nil
}

func appleVisionExecutable(ctx context.Context) (string, error) {
	appleVisionCompileMu.Lock()
	defer appleVisionCompileMu.Unlock()

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}
	cacheDir = filepath.Join(cacheDir, "clip-indexer", "vision")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("could not create Apple Vision cache: %w", err)
	}

	digest := sha256.Sum256([]byte(appleVisionSource))
	version := hex.EncodeToString(digest[:8])
	executable := filepath.Join(cacheDir, "apple-vision-"+version)
	if info, err := os.Stat(executable); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
		return executable, nil
	}

	sourcePath := filepath.Join(cacheDir, "apple-vision-"+version+".swift")
	if err := os.WriteFile(sourcePath, []byte(appleVisionSource), 0o644); err != nil {
		return "", fmt.Errorf("could not cache Apple Vision source: %w", err)
	}
	temporaryExecutable := fmt.Sprintf("%s-%d.tmp", executable, os.Getpid())
	defer os.Remove(temporaryExecutable)

	swiftc, err := exec.LookPath("swiftc")
	if err != nil {
		return "", errors.New("local vision mode requires the macOS Swift toolchain (swiftc)")
	}
	output, err := exec.CommandContext(ctx, swiftc, "-O", sourcePath, "-o", temporaryExecutable).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("could not compile Apple Vision helper: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if err := os.Rename(temporaryExecutable, executable); err != nil {
		if info, statErr := os.Stat(executable); statErr == nil && info.Mode().Perm()&0o111 != 0 {
			return executable, nil
		}
		return "", fmt.Errorf("could not install Apple Vision helper: %w", err)
	}
	return executable, nil
}

func localVisionFramePaths(frames []visionFrame) ([]string, func(), error) {
	allMaterialized := true
	paths := make([]string, len(frames))
	for index, frame := range frames {
		if strings.TrimSpace(frame.Path) == "" {
			allMaterialized = false
			break
		}
		paths[index] = frame.Path
	}
	if allMaterialized {
		return paths, nil, nil
	}

	tempDir, err := os.MkdirTemp("", "clip-indexer-local-vision-*")
	if err != nil {
		return nil, nil, fmt.Errorf("could not create local vision frame directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tempDir) }
	for index, frame := range frames {
		path := filepath.Join(tempDir, fmt.Sprintf("frame_%02d.jpg", index+1))
		if err := os.WriteFile(path, frame.Data, 0o600); err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("could not materialize local vision frame: %w", err)
		}
		paths[index] = path
	}
	return paths, cleanup, nil
}
