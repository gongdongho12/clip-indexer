package media

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type exportOptions struct {
	OutputDir    string
	IncludeMedia bool
	Overwrite    bool
}

type exportManifest struct {
	Service     ServiceInfo  `json:"service"`
	GeneratedAt string       `json:"generated_at"`
	ReportPath  string       `json:"report_path"`
	FilesPath   string       `json:"files_path"`
	HTMLPath    string       `json:"html_path"`
	Files       []exportFile `json:"files"`
}

type exportFile struct {
	SourcePath       string   `json:"source_path"`
	ExportPath       string   `json:"export_path,omitempty"`
	MediaType        string   `json:"media_type"`
	OriginalFileName string   `json:"original_file_name"`
	FinalFileName    string   `json:"final_file_name"`
	ShotAt           string   `json:"shot_at,omitempty"`
	Folder           string   `json:"folder,omitempty"`
	Tags             []string `json:"tags,omitempty"`
}

type exportConflictError struct {
	Paths []string
}

func (err exportConflictError) Error() string {
	if len(err.Paths) == 1 {
		return fmt.Sprintf("export target already exists: %s", err.Paths[0])
	}
	paths := err.Paths
	if len(paths) > 4 {
		paths = paths[:4]
	}
	message := fmt.Sprintf("export targets already exist (%d): %s", len(err.Paths), strings.Join(paths, ", "))
	if len(err.Paths) > len(paths) {
		message += ", ..."
	}
	return message
}

func runExport(args []string, stdin io.Reader, stdout, stderr io.Writer, envWarnings []string) error {
	cfg := defaultConfig()
	options := exportOptions{}

	fs := flag.NewFlagSet(cliName+" export", flag.ContinueOnError)
	fs.SetOutput(stderr)
	addIndexFlags(fs, &cfg)
	fs.StringVar(&options.OutputDir, "out-dir", "", "directory for static export; default tmp/clip-atlas-export/<timestamp>")
	fs.BoolVar(&options.IncludeMedia, "include-media", false, "copy source media into the export bundle")
	fs.BoolVar(&options.Overwrite, "overwrite", false, "overwrite existing export files without prompting")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: %s export [flags] <media-file-or-directory>...\n\n", cliName)
		fmt.Fprintln(stderr, "Writes index.html, report.json, and files.json for serverless viewing.")
		fmt.Fprintln(stderr, "\nFlags:")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return errors.New("at least one file or directory is required")
	}
	if options.OutputDir == "" {
		options.OutputDir = defaultExportOutputDir()
	}

	report, err := BuildReport(context.Background(), cfg, fs.Args())
	if err != nil {
		return err
	}
	report.Warnings = append(envWarnings, report.Warnings...)
	refreshReportDerived(&report, reportFilesDiscovered(report))

	manifest, err := writeStaticExport(report, options)
	var conflictErr exportConflictError
	if errors.As(err, &conflictErr) && !options.Overwrite {
		confirmed, promptErr := confirmExportOverwrite(stdin, stderr, conflictErr.Paths)
		if promptErr != nil {
			return promptErr
		}
		if !confirmed {
			return errors.New("export canceled")
		}
		options.Overwrite = true
		manifest, err = writeStaticExport(report, options)
	}
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "Export HTML: %s\n", manifest.HTMLPath)
	fmt.Fprintf(stdout, "Report JSON: %s\n", manifest.ReportPath)
	fmt.Fprintf(stdout, "Files JSON: %s\n", manifest.FilesPath)
	return nil
}

func defaultExportOutputDir() string {
	return filepath.Join("tmp", "clip-atlas-export", time.Now().Format("20060102-150405"))
}

func writeStaticExport(report Report, options exportOptions) (exportManifest, error) {
	absOutputDir, err := filepath.Abs(options.OutputDir)
	if err != nil {
		return exportManifest{}, err
	}
	if !options.Overwrite {
		if conflicts := existingExportTargets(report.Items, absOutputDir, options.IncludeMedia); len(conflicts) > 0 {
			return exportManifest{}, exportConflictError{Paths: conflicts}
		}
	}
	if err := os.MkdirAll(absOutputDir, 0o755); err != nil {
		return exportManifest{}, err
	}

	files, err := exportFiles(report.Items, absOutputDir, options.IncludeMedia)
	if err != nil {
		return exportManifest{}, err
	}
	manifest := exportManifest{
		Service:     ServiceInfo{Name: serviceName, CLI: cliName, Version: version},
		GeneratedAt: time.Now().Format(time.RFC3339),
		ReportPath:  filepath.Join(absOutputDir, "report.json"),
		FilesPath:   filepath.Join(absOutputDir, "files.json"),
		HTMLPath:    filepath.Join(absOutputDir, "index.html"),
		Files:       files,
	}
	if err := writePrettyJSONFile(manifest.ReportPath, report); err != nil {
		return exportManifest{}, err
	}
	if err := writePrettyJSONFile(manifest.FilesPath, files); err != nil {
		return exportManifest{}, err
	}
	if err := os.WriteFile(filepath.Join(absOutputDir, "manifest.json"), mustJSON(manifest), 0o644); err != nil {
		return exportManifest{}, err
	}
	if err := os.WriteFile(manifest.HTMLPath, []byte(renderStaticExportHTML(report, files)), 0o644); err != nil {
		return exportManifest{}, err
	}
	return manifest, nil
}

func existingExportTargets(items []Item, outputDir string, includeMedia bool) []string {
	targets := []string{
		filepath.Join(outputDir, "index.html"),
		filepath.Join(outputDir, "report.json"),
		filepath.Join(outputDir, "files.json"),
		filepath.Join(outputDir, "manifest.json"),
	}
	if includeMedia {
		for index, item := range items {
			name := fmt.Sprintf("%04d_%s", index+1, sanitizeExportFileName(filepath.Base(item.SourcePath)))
			targets = append(targets, filepath.Join(outputDir, "media", name))
		}
	}
	conflicts := make([]string, 0, len(targets))
	for _, target := range targets {
		if _, err := os.Stat(target); err == nil {
			conflicts = append(conflicts, target)
		}
	}
	return conflicts
}

func confirmExportOverwrite(stdin io.Reader, stderr io.Writer, paths []string) (bool, error) {
	fmt.Fprintln(stderr, "Export target files already exist:")
	for index, path := range paths {
		if index >= 10 {
			fmt.Fprintf(stderr, "  ... and %d more\n", len(paths)-index)
			break
		}
		fmt.Fprintf(stderr, "  %s\n", path)
	}
	fmt.Fprint(stderr, "Overwrite existing export files? [y/N] ")

	answer, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

func exportFiles(items []Item, outputDir string, includeMedia bool) ([]exportFile, error) {
	files := make([]exportFile, 0, len(items))
	for index, item := range items {
		file := exportFile{
			SourcePath:       item.SourcePath,
			MediaType:        itemMediaType(item),
			OriginalFileName: item.OriginalFileName,
			FinalFileName:    item.FinalFileName,
			ShotAt:           item.ShotAt,
			Tags:             append([]string{}, item.Tags...),
		}
		if item.Group != nil {
			file.Folder = item.Group.Folder
		}
		if includeMedia {
			relative, err := copyExportMedia(item.SourcePath, outputDir, index)
			if err != nil {
				return nil, err
			}
			file.ExportPath = relative
		}
		files = append(files, file)
	}
	return files, nil
}

func copyExportMedia(sourcePath string, outputDir string, index int) (string, error) {
	source, err := os.Open(sourcePath)
	if err != nil {
		return "", err
	}
	defer source.Close()

	mediaDir := filepath.Join(outputDir, "media")
	if err := os.MkdirAll(mediaDir, 0o755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%04d_%s", index+1, sanitizeExportFileName(filepath.Base(sourcePath)))
	targetPath := filepath.Join(mediaDir, name)
	target, err := os.Create(targetPath)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(target, source); err != nil {
		_ = target.Close()
		return "", err
	}
	if err := target.Close(); err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Join("media", name)), nil
}

func sanitizeExportFileName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "media"
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "\x00", "_")
	return replacer.Replace(name)
}

func renderStaticExportHTML(report Report, files []exportFile) string {
	reportJSON := string(mustJSON(report))
	filesJSON := string(mustJSON(files))
	return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Clip Atlas Export</title>
  <style>
    :root { color-scheme: light; --line:#dbe4e7; --ink:#172026; --muted:#61717b; --accent:#137f83; --bg:#f6f8f9; }
    * { box-sizing: border-box; }
    body { margin: 0; font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; color: var(--ink); background: var(--bg); }
    header { position: sticky; top: 0; z-index: 2; display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 16px; align-items: center; padding: 16px 20px; border-bottom: 1px solid var(--line); background: #fff; }
    h1 { margin: 0; font-size: 20px; letter-spacing: 0; }
    .sub { margin-top: 4px; color: var(--muted); font-size: 13px; }
    .stats { display: flex; flex-wrap: wrap; gap: 8px; justify-content: flex-end; }
    .stat { min-width: 78px; padding: 6px 9px; border: 1px solid var(--line); border-radius: 7px; background: #fbfdfd; color: var(--muted); font-size: 12px; }
    .stat strong { display:block; color: var(--ink); font-size: 16px; }
    main { display: grid; grid-template-columns: minmax(230px, 300px) minmax(0, 1fr) minmax(320px, 420px); min-height: calc(100vh - 78px); }
    .nav { min-width: 0; overflow: auto; padding: 12px; border-right: 1px solid var(--line); background:#fff; }
    .nav-section { display:grid; gap:8px; margin-bottom:18px; }
    .nav-title { color: var(--muted); font-size: 12px; font-weight: 800; text-transform: uppercase; letter-spacing: 0; }
    .folder-node, .tag-node { width:100%; min-height:30px; display:grid; grid-template-columns:minmax(0, 1fr) auto; gap:8px; align-items:center; border:1px solid var(--line); border-radius:7px; background:#fbfdfd; color:var(--ink); padding:5px 8px; font:inherit; text-align:left; cursor:pointer; }
    .folder-node:hover, .tag-node:hover { border-color: var(--accent); }
    .folder-node.active, .tag-node.active { border-color: var(--accent); background:#eaf7f6; }
    .node-label { overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
    .node-count { color:var(--muted); font-size:12px; }
    .list { min-width: 0; overflow: auto; }
    .toolbar { position: sticky; top: 0; z-index: 1; display:grid; grid-template-columns:minmax(0,1fr) auto; gap:8px; padding: 12px; border-bottom: 1px solid var(--line); background: #fff; }
    input, button { min-height: 36px; padding: 7px 10px; border: 1px solid var(--line); border-radius: 7px; font: inherit; background:#fff; color:var(--ink); }
    input { width: 100%; }
    table { width: 100%; border-collapse: collapse; table-layout: fixed; background: #fff; }
    th, td { padding: 9px 10px; border-bottom: 1px solid var(--line); text-align: left; vertical-align: top; }
    th { position: sticky; top: 61px; z-index: 1; background: #f8fafb; color: var(--muted); font-size: 12px; }
    tr { cursor: pointer; }
    tr:hover, tr.active { background: #eef8f7; }
    .name { font-weight: 700; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .path, .tags, .muted, .description-cell { color: var(--muted); font-size: 12px; overflow-wrap: anywhere; }
    .description-cell { line-height: 1.35; }
    .actions { display:flex; flex-wrap:wrap; gap:6px; align-items:center; }
    .actions a, .actions button { min-height:30px; padding:5px 8px; border:1px solid var(--line); border-radius:6px; background:#fff; color:var(--ink); font:inherit; font-size:12px; text-decoration:none; cursor:pointer; }
    .actions a:hover, .actions button:hover { border-color: var(--accent); }
    .side { position: sticky; top: 78px; align-self: start; height: calc(100vh - 78px); display: grid; grid-template-rows: minmax(220px, 38vh) minmax(0, 1fr); border-left: 1px solid var(--line); background: #fff; }
    .preview { display:grid; grid-template-rows:minmax(0, 1fr) auto; min-height:0; background:#101820; color:#eff6f7; }
    .preview video, .preview img { width:100%; height:100%; object-fit:contain; min-height:0; }
    .audio-box { display:grid; gap:14px; place-items:center; padding:18px; text-align:center; }
    audio { width:min(92%, 520px); }
    .preview-title { padding:8px 12px; background:#101820; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
    .detail { min-height:0; overflow:auto; padding:14px; }
    .detail h2 { margin:0 0 10px; font-size:16px; }
    .detail-section { padding:12px 0; border-top:1px solid var(--line); }
    .detail-section:first-of-type { margin-top:10px; }
    .detail-section h3 { margin:0 0 8px; color:var(--muted); font-size:12px; font-weight:800; text-transform:uppercase; letter-spacing:0; }
    .detail-label { margin:10px 0 4px; color:var(--muted); font-size:12px; font-weight:800; }
    .detail-text { margin:0 0 8px; color:var(--ink); font-size:13px; line-height:1.45; white-space:pre-wrap; overflow-wrap:anywhere; }
    .transcript { margin:8px 0 0; }
    .transcript summary { cursor:pointer; color:var(--accent); font-size:13px; font-weight:700; }
    .pills { display:flex; flex-wrap:wrap; gap:6px; }
    .pill { padding:4px 7px; border:1px solid var(--line); border-radius:999px; background:#f8fbfb; color:var(--ink); font-size:12px; }
    .raw-json { margin-top:4px; }
    .raw-json summary { cursor:pointer; color:var(--accent); font-size:13px; font-weight:700; }
    .raw-json pre { max-height:320px; overflow:auto; margin:8px 0 0; padding:10px; border:1px solid var(--line); border-radius:7px; background:#f8fafb; color:var(--ink); font-size:12px; line-height:1.4; white-space:pre-wrap; }
    .grid { display:grid; grid-template-columns: 112px minmax(0, 1fr); gap:7px 10px; font-size:13px; color:var(--muted); }
    .grid strong { color:var(--ink); overflow-wrap:anywhere; }
    @media (max-width: 1060px) { main { grid-template-columns: minmax(0, 1fr); } .nav { max-height: 320px; border-right:0; border-bottom:1px solid var(--line); } .side { position:static; height:auto; border-left:0; border-top:1px solid var(--line); } th { top:0; } table { min-width: 760px; } }
  </style>
</head>
<body>
  <header>
    <div><h1>Clip Atlas Export</h1><div class="sub" id="subtitle"></div></div>
    <div class="stats" id="stats"></div>
  </header>
  <main>
    <aside class="nav">
      <section class="nav-section">
        <div class="nav-title">Folders</div>
        <div id="folderMap"></div>
      </section>
      <section class="nav-section">
        <div class="nav-title">Tag Map</div>
        <div id="tagMap"></div>
      </section>
    </aside>
    <section class="list">
      <div class="toolbar"><input id="filter" type="search" placeholder="Filter files, tags, folders"><button id="clearFilter" type="button">Clear</button></div>
      <table>
        <thead><tr><th style="width:24%">File</th><th style="width:9%">Media</th><th style="width:14%">Shot</th><th style="width:22%">Description</th><th>Tags</th><th style="width:14%">Final</th><th style="width:11%">Actions</th></tr></thead>
        <tbody id="rows"></tbody>
      </table>
    </section>
    <aside class="side">
      <div class="preview" id="preview"><div class="audio-box">Select a file</div></div>
      <section class="detail" id="detail"></section>
    </aside>
  </main>
  <script id="report-data" type="application/json">` + escapeScriptJSON(reportJSON) + `</script>
  <script id="files-data" type="application/json">` + escapeScriptJSON(filesJSON) + `</script>
  <script>
    const report = JSON.parse(document.getElementById("report-data").textContent);
    const files = JSON.parse(document.getElementById("files-data").textContent);
    let active = files[0]?.source_path || "";
    let selectedFolder = "";
    let selectedTag = "";
    const $ = (id) => document.getElementById(id);
    function esc(value) { return String(value ?? "").replace(/[&<>"']/g, (char) => ({ "&":"&amp;", "<":"&lt;", ">":"&gt;", "\"":"&quot;", "'":"&#39;" }[char])); }
    function base(path) { return String(path || "").replace(/\\/g, "/").split("/").pop() || path; }
    function shot(value) { if (!value) return "-"; const date = new Date(value); return Number.isNaN(date.valueOf()) ? value : date.toLocaleString(); }
    function item(path) { return files.find((file) => file.source_path === path); }
    function reportItem(path) { return (report.items || []).find((file) => file.source_path === path); }
    function compact(values) {
      const output = [];
      const seen = new Set();
      for (const value of values || []) {
        const text = String(value ?? "").trim();
        if (!text || seen.has(text)) continue;
        seen.add(text);
        output.push(text);
      }
      return output;
    }
    function firstText(values) {
      return compact(values)[0] || "";
    }
    function clipText(value, max) {
      const text = String(value ?? "").trim().replace(/\s+/g, " ");
      if (!text) return "";
      return text.length > max ? text.slice(0, Math.max(0, max - 3)) + "..." : text;
    }
    function confidence(value) {
      if (value === undefined || value === null || value === "") return "";
      const number = Number(value);
      if (!Number.isFinite(number)) return "";
      return Math.round(number * 100) + "%";
    }
    function coordinates(location) {
      if (!location || !Number.isFinite(location.latitude) || !Number.isFinite(location.longitude)) return "";
      return location.latitude.toFixed(6) + ", " + location.longitude.toFixed(6);
    }
    function allTags(file, source) {
      return compact([...(file?.tags || []), ...(source?.content?.tags || []), ...(source?.content?.audio_tags || [])]);
    }
    function descriptionText(source) {
      const content = source?.content || {};
      return firstText([content.scene_summary, content.audio_summary, content.notes, source?.llm_notes, content.audio_transcript]);
    }
    function searchableText(file, source) {
      const content = source?.content || {};
      const location = source?.location || {};
      return [
        file.source_path,
        file.original_file_name,
        file.final_file_name,
        file.folder,
        allTags(file, source).join(" "),
        content.scene_summary,
        content.audio_summary,
        content.audio_transcript,
        content.location_guess,
        content.notes,
        content.model,
        content.audio_model,
        source?.llm_notes,
        source?.group?.label,
        source?.group?.folder,
        source?.group?.reason,
        location.label,
        location.source,
        location.notes,
        (source?.warnings || []).join(" ")
      ].join(" ").toLowerCase();
    }
    function stats() {
      const summary = report.summary || {};
      const descriptions = (report.items || []).filter((source) => descriptionText(source)).length;
      $("subtitle").textContent = report.options?.trip || "Static media report";
      $("stats").innerHTML = [["Files", summary.files_indexed || files.length], ["Images", summary.with_image_file || 0], ["Video", summary.with_video_stream || 0], ["Audio", summary.with_audio_stream || 0], ["Scene", summary.with_content || 0], ["Descriptions", descriptions]].map(([label, value]) => "<div class=\"stat\"><strong>" + esc(value) + "</strong>" + esc(label) + "</div>").join("");
    }
    function mediaElement(file) {
      const src = file.export_path || "";
      if (!src) return "<div class=\"audio-box\"><strong>" + esc(base(file.source_path)) + "</strong><span class=\"muted\">" + esc(file.source_path) + "</span></div>";
      if (file.media_type === "image") return "<img src=\"" + esc(src) + "\" alt=\"" + esc(base(file.source_path)) + "\">";
      if (file.media_type === "audio") return "<div class=\"audio-box\"><strong>" + esc(base(file.source_path)) + "</strong><audio controls src=\"" + esc(src) + "\"></audio></div>";
      return "<video controls preload=\"metadata\" src=\"" + esc(src) + "\"></video>";
    }
    function flattenFolders(nodes, output = []) {
      for (const node of nodes || []) {
        output.push(node);
        flattenFolders(node.children || [], output);
      }
      return output;
    }
    function tagEntries() {
      const counts = new Map();
      for (const file of files) {
        const source = reportItem(file.source_path) || {};
        for (const tag of allTags(file, source)) counts.set(tag, (counts.get(tag) || 0) + 1);
      }
      return [...counts.entries()].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0])).slice(0, 80);
    }
    function actionHTML(file, includeJSON = false) {
      const open = file.export_path ? "<a href=\"" + esc(file.export_path) + "\" target=\"_blank\" rel=\"noopener\">Open</a>" : "";
      const json = includeJSON ? "<button type=\"button\" data-copy-json=\"" + esc(file.source_path) + "\">Copy JSON</button>" : "";
      return "<div class=\"actions\">" + open + "<button type=\"button\" data-copy-path=\"" + esc(file.source_path) + "\">Copy path</button>" + json + "</div>";
    }
    async function copyText(label, value) {
      try {
        if (!navigator.clipboard) throw new Error("clipboard unavailable");
        await navigator.clipboard.writeText(value);
      } catch {
        window.prompt(label, value);
      }
    }
    function folderMatches(file) {
      if (!selectedFolder) return true;
      return String(file.source_path || "").startsWith(selectedFolder);
    }
    function tagMatches(file) {
      if (!selectedTag) return true;
      const source = reportItem(file.source_path) || {};
      return allTags(file, source).includes(selectedTag);
    }
    function filteredFiles() {
      const query = $("filter").value.trim().toLowerCase();
      return files.filter((file) => {
        if (!folderMatches(file) || !tagMatches(file)) return false;
        if (!query) return true;
        const source = reportItem(file.source_path) || {};
        return searchableText(file, source).includes(query);
      });
    }
    function renderMaps() {
      const folders = flattenFolders(report.folder_tree || []);
      $("folderMap").innerHTML = folders.length
        ? folders.map((folder) => "<button class=\"folder-node " + (folder.path === selectedFolder ? "active" : "") + "\" data-folder=\"" + esc(folder.path) + "\" style=\"margin-left:" + (Math.min(folder.depth || 0, 7) * 10) + "px\"><span class=\"node-label\">" + esc(folder.relative_path || folder.name || folder.path) + "</span><span class=\"node-count\">" + esc(folder.total_file_count || folder.file_count || 0) + "</span></button>").join("")
        : "<div class=\"muted\">No folder tree</div>";
      $("tagMap").innerHTML = tagEntries().map(([tag, count]) => "<button class=\"tag-node " + (tag === selectedTag ? "active" : "") + "\" data-tag=\"" + esc(tag) + "\"><span class=\"node-label\">" + esc(tag) + "</span><span class=\"node-count\">" + esc(count) + "</span></button>").join("") || "<div class=\"muted\">No tags</div>";
      document.querySelectorAll("[data-folder]").forEach((button) => button.addEventListener("click", () => {
        selectedFolder = selectedFolder === button.dataset.folder ? "" : button.dataset.folder;
        render();
      }));
      document.querySelectorAll("[data-tag]").forEach((button) => button.addEventListener("click", () => {
        selectedTag = selectedTag === button.dataset.tag ? "" : button.dataset.tag;
        render();
      }));
    }
    function textBlock(value) {
      const text = String(value ?? "").trim();
      return text ? "<p class=\"detail-text\">" + esc(text) + "</p>" : "";
    }
    function labeledText(label, value) {
      const text = String(value ?? "").trim();
      if (!text) return "";
      return "<div class=\"detail-label\">" + esc(label) + "</div>" + textBlock(text);
    }
    function detailSection(title, body) {
      return body ? "<section class=\"detail-section\"><h3>" + esc(title) + "</h3>" + body + "</section>" : "";
    }
    function detailGrid(rows) {
      const cells = [];
      for (const row of rows) {
        const value = Array.isArray(row[1]) ? compact(row[1]).join(", ") : String(row[1] ?? "").trim();
        if (!value) continue;
        cells.push("<span>" + esc(row[0]) + "</span><strong>" + esc(value) + "</strong>");
      }
      return cells.length ? "<div class=\"grid\">" + cells.join("") + "</div>" : "";
    }
    function pillList(values) {
      const tags = compact(values);
      return tags.length ? "<div class=\"pills\">" + tags.map((tag) => "<span class=\"pill\">" + esc(tag) + "</span>").join("") + "</div>" : "";
    }
    function transcriptBlock(content) {
      const transcript = String(content?.audio_transcript || "").trim();
      if (!transcript) return "";
      return "<details class=\"transcript\" open><summary>Audio Transcript</summary>" + textBlock(transcript) + "</details>";
    }
    function analysisMeta(source) {
      const content = source?.content || {};
      return detailGrid([
        ["Frame Count", content.frame_count],
        ["Audio Seconds", content.audio_seconds],
        ["Vision Model", content.model],
        ["Audio Model", content.audio_model],
        ["Item Confidence", confidence(source?.confidence)],
        ["Location Confidence", confidence(content.location_confidence)]
      ]);
    }
    function locationDetail(source) {
      const content = source?.content || {};
      const location = source?.location || {};
      return detailGrid([
        ["Label", location.label || content.location_guess],
        ["Coordinates", coordinates(location)],
        ["Source", location.source],
        ["Confidence", confidence(location.confidence !== undefined && location.confidence !== null ? location.confidence : content.location_confidence)],
        ["Notes", location.notes]
      ]);
    }
    function rawJSON(source) {
      if (!source || !source.source_path) return "";
      return "<details class=\"raw-json\"><summary>View item JSON</summary><pre>" + esc(JSON.stringify(source, null, 2)) + "</pre></details>";
    }
    function renderRows() {
      const rows = filteredFiles();
      $("rows").innerHTML = rows.map((file) => {
        const source = reportItem(file.source_path) || {};
        return "<tr data-path=\"" + esc(file.source_path) + "\" class=\"" + (file.source_path === active ? "active" : "") + "\"><td><div class=\"name\">" + esc(base(file.source_path)) + "</div><div class=\"path\">" + esc(file.source_path) + "</div></td><td>" + esc(file.media_type || "-") + "</td><td>" + esc(shot(file.shot_at)) + "</td><td class=\"description-cell\">" + esc(clipText(descriptionText(source), 180) || "-") + "</td><td class=\"tags\">" + esc(allTags(file, source).join(", ")) + "</td><td>" + esc(file.final_file_name || "") + "</td><td>" + actionHTML(file) + "</td></tr>";
      }).join("");
      document.querySelectorAll("tr[data-path]").forEach((row) => row.addEventListener("click", (event) => {
        if (event.target.closest("button,a")) return;
        active = row.dataset.path;
        render();
      }));
    }
    function renderDetail() {
      const file = item(active);
      const source = reportItem(active) || {};
      if (!file) { $("preview").innerHTML = "<div class=\"audio-box\">Select a file</div>"; $("detail").innerHTML = ""; return; }
      $("preview").innerHTML = mediaElement(file) + "<div class=\"preview-title\">" + esc(base(file.source_path)) + "</div>";
      const content = source.content || {};
      const description = labeledText("Scene Description", content.scene_summary) + labeledText("Notes", content.notes) + labeledText("LLM Notes", source.llm_notes);
      const audio = labeledText("Audio Summary", content.audio_summary) + transcriptBlock(content) + labeledText("Audio Tags", compact(content.audio_tags || []).join(", "));
      const tags = pillList(allTags(file, source)) + labeledText("Content Tags", compact(content.tags || []).join(", ")) + labeledText("Audio Tags", compact(content.audio_tags || []).join(", "));
      const grouping = detailGrid([
        ["Folder", file.folder || source.group?.folder],
        ["Group Label", source.group?.label],
        ["Group Key", source.group?.key],
        ["Group Reason", source.group?.reason]
      ]);
      const fileInfo = detailGrid([
        ["Media", file.media_type || source.media_type],
        ["Original", file.original_file_name || source.original_file_name],
        ["Final", file.final_file_name || source.final_file_name],
        ["Recommended", source.recommended_file_name],
        ["Shot", shot(file.shot_at || source.shot_at)],
        ["Duration", source.duration_seconds ? String(source.duration_seconds) + "s" : ""],
        ["Video", source.video ? compact([source.video.codec, source.video.width && source.video.height ? source.video.width + "x" + source.video.height : "", source.video.fps ? Math.round(source.video.fps) + "fps" : ""]).join(" ") : ""],
        ["Audio", source.audio ? compact([source.audio.codec, source.audio.channels ? source.audio.channels + "ch" : "", source.audio.sample_rate ? source.audio.sample_rate + "Hz" : ""]).join(" ") : ""],
        ["Source", file.source_path],
        ["Export", file.export_path || "-"]
      ]);
      const warnings = (source.warnings || []).length ? "<ul>" + source.warnings.map((warning) => "<li>" + esc(warning) + "</li>").join("") + "</ul>" : "";
      $("detail").innerHTML = "<h2>" + esc(base(file.source_path)) + "</h2>" + actionHTML(file, true)
        + detailSection("Description", description || textBlock("-"))
        + detailSection("Audio", audio)
        + detailSection("Tags", tags)
        + detailSection("Analysis", analysisMeta(source))
        + detailSection("Location", locationDetail(source))
        + detailSection("Grouping", grouping)
        + detailSection("Warnings", warnings)
        + detailSection("File", fileInfo)
        + detailSection("Raw JSON", rawJSON(source));
    }
    function render() { stats(); renderMaps(); renderRows(); renderDetail(); }
    $("filter").addEventListener("input", renderRows);
    $("clearFilter").addEventListener("click", () => { $("filter").value = ""; selectedFolder = ""; selectedTag = ""; render(); });
    document.addEventListener("click", (event) => {
      const pathButton = event.target.closest("[data-copy-path]");
      const jsonButton = event.target.closest("[data-copy-json]");
      if (!pathButton && !jsonButton) return;
      event.stopPropagation();
      if (pathButton) copyText("Copy path", pathButton.dataset.copyPath || "");
      if (jsonButton) {
        const source = reportItem(jsonButton.dataset.copyJson || "");
        copyText("Copy item JSON", JSON.stringify(source || {}, null, 2));
      }
    });
    render();
  </script>
</body>
</html>
`
}

func mustJSON(value any) []byte {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		panic(err)
	}
	return append(data, '\n')
}

func escapeScriptJSON(value string) string {
	value = strings.ReplaceAll(value, "</", "<\\/")
	value = strings.ReplaceAll(value, "\u2028", "\\u2028")
	value = strings.ReplaceAll(value, "\u2029", "\\u2029")
	return value
}
