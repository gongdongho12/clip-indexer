package media

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteStaticExportIncludesFolderAndTagViews(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "photo.jpg")
	if err := os.WriteFile(source, []byte("image"), 0o644); err != nil {
		t.Fatal(err)
	}
	report := Report{
		Service: ServiceInfo{Name: serviceName, CLI: cliName, Version: version},
		Items: []Item{{
			SourcePath:       source,
			MediaType:        mediaTypeImage,
			OriginalFileName: "photo.jpg",
			Extension:        ".jpg",
			Tags:             []string{"image", "street"},
			FinalFileName:    "photo.jpg",
			Content:          &ContentInfo{SceneSummary: "Street photo", Tags: []string{"street"}},
		}},
	}
	refreshReportDerived(&report, len(report.Items))

	outputDir := filepath.Join(dir, "export")
	manifest, err := writeStaticExport(report, exportOptions{OutputDir: outputDir, IncludeMedia: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(manifest.HTMLPath); err != nil {
		t.Fatalf("expected export HTML: %v", err)
	}
	if len(manifest.Files) != 1 || manifest.Files[0].ExportPath == "" {
		t.Fatalf("expected copied media in manifest, got %#v", manifest.Files)
	}
	if _, err := os.Stat(filepath.Join(outputDir, filepath.FromSlash(manifest.Files[0].ExportPath))); err != nil {
		t.Fatalf("expected copied media file: %v", err)
	}
	data, err := os.ReadFile(manifest.HTMLPath)
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, expected := range []string{"folderMap", "tagMap", "Street photo"} {
		if !strings.Contains(html, expected) {
			t.Fatalf("expected export HTML to contain %q", expected)
		}
	}
	var files []exportFile
	filesData, err := os.ReadFile(manifest.FilesPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(filesData, &files); err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].MediaType != mediaTypeImage {
		t.Fatalf("unexpected files json: %#v", files)
	}
}
