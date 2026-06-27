package media

import (
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

func runExport(args []string, stdout, stderr io.Writer, envWarnings []string) error {
	cfg := defaultConfig()
	options := exportOptions{}

	fs := flag.NewFlagSet(cliName+" export", flag.ContinueOnError)
	fs.SetOutput(stderr)
	addIndexFlags(fs, &cfg)
	fs.StringVar(&options.OutputDir, "out-dir", "", "directory for static export; default tmp/clip-atlas-export/<timestamp>")
	fs.BoolVar(&options.IncludeMedia, "include-media", false, "copy source media into the export bundle")
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
		options.OutputDir = filepath.Join("tmp", "clip-atlas-export", time.Now().Format("20060102-150405"))
	}

	report, err := BuildReport(context.Background(), cfg, fs.Args())
	if err != nil {
		return err
	}
	report.Warnings = append(envWarnings, report.Warnings...)
	refreshReportDerived(&report, reportFilesDiscovered(report))

	manifest, err := writeStaticExport(report, options)
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "Export HTML: %s\n", manifest.HTMLPath)
	fmt.Fprintf(stdout, "Report JSON: %s\n", manifest.ReportPath)
	fmt.Fprintf(stdout, "Files JSON: %s\n", manifest.FilesPath)
	return nil
}

func writeStaticExport(report Report, options exportOptions) (exportManifest, error) {
	absOutputDir, err := filepath.Abs(options.OutputDir)
	if err != nil {
		return exportManifest{}, err
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
    .path, .tags, .muted { color: var(--muted); font-size: 12px; overflow-wrap: anywhere; }
    .side { position: sticky; top: 78px; align-self: start; height: calc(100vh - 78px); display: grid; grid-template-rows: minmax(220px, 38vh) minmax(0, 1fr); border-left: 1px solid var(--line); background: #fff; }
    .preview { display:grid; grid-template-rows:minmax(0, 1fr) auto; min-height:0; background:#101820; color:#eff6f7; }
    .preview video, .preview img { width:100%; height:100%; object-fit:contain; min-height:0; }
    .audio-box { display:grid; gap:14px; place-items:center; padding:18px; text-align:center; }
    audio { width:min(92%, 520px); }
    .preview-title { padding:8px 12px; background:#101820; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
    .detail { min-height:0; overflow:auto; padding:14px; }
    .detail h2 { margin:0 0 10px; font-size:16px; }
    .grid { display:grid; grid-template-columns: 96px minmax(0, 1fr); gap:7px 10px; font-size:13px; color:var(--muted); }
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
        <thead><tr><th style="width:30%">File</th><th style="width:12%">Media</th><th style="width:18%">Shot</th><th>Tags</th><th style="width:18%">Final</th></tr></thead>
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
    function stats() {
      const summary = report.summary || {};
      $("subtitle").textContent = report.options?.trip || "Static media report";
      $("stats").innerHTML = [["Files", summary.files_indexed || files.length], ["Images", summary.with_image_file || 0], ["Video", summary.with_video_stream || 0], ["Audio", summary.with_audio_stream || 0], ["Scene", summary.with_content || 0]].map(([label, value]) => "<div class=\"stat\"><strong>" + esc(value) + "</strong>" + esc(label) + "</div>").join("");
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
        const item = reportItem(file.source_path) || {};
        const tags = new Set([...(file.tags || []), ...(item.content?.tags || []), ...(item.content?.audio_tags || [])].filter(Boolean));
        for (const tag of tags) counts.set(tag, (counts.get(tag) || 0) + 1);
      }
      return [...counts.entries()].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0])).slice(0, 80);
    }
    function folderMatches(file) {
      if (!selectedFolder) return true;
      return String(file.source_path || "").startsWith(selectedFolder);
    }
    function tagMatches(file) {
      if (!selectedTag) return true;
      const item = reportItem(file.source_path) || {};
      const tags = [...(file.tags || []), ...(item.content?.tags || []), ...(item.content?.audio_tags || [])];
      return tags.includes(selectedTag);
    }
    function filteredFiles() {
      const query = $("filter").value.trim().toLowerCase();
      return files.filter((file) => {
        if (!folderMatches(file) || !tagMatches(file)) return false;
        if (!query) return true;
        const item = reportItem(file.source_path) || {};
        return [
          file.source_path,
          file.final_file_name,
          file.folder,
          (file.tags || []).join(" "),
          item.content?.scene_summary || "",
          item.content?.audio_summary || "",
          item.content?.audio_transcript || "",
          item.content?.location_guess || ""
        ].join(" ").toLowerCase().includes(query);
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
    function renderRows() {
      const rows = filteredFiles();
      $("rows").innerHTML = rows.map((file) => "<tr data-path=\"" + esc(file.source_path) + "\" class=\"" + (file.source_path === active ? "active" : "") + "\"><td><div class=\"name\">" + esc(base(file.source_path)) + "</div><div class=\"path\">" + esc(file.source_path) + "</div></td><td>" + esc(file.media_type || "-") + "</td><td>" + esc(shot(file.shot_at)) + "</td><td class=\"tags\">" + esc((file.tags || []).join(", ")) + "</td><td>" + esc(file.final_file_name || "") + "</td></tr>").join("");
      document.querySelectorAll("tr[data-path]").forEach((row) => row.addEventListener("click", () => { active = row.dataset.path; render(); }));
    }
    function renderDetail() {
      const file = item(active);
      const source = reportItem(active) || {};
      if (!file) { $("preview").innerHTML = "<div class=\"audio-box\">Select a file</div>"; $("detail").innerHTML = ""; return; }
      $("preview").innerHTML = mediaElement(file) + "<div class=\"preview-title\">" + esc(base(file.source_path)) + "</div>";
      $("detail").innerHTML = "<h2>" + esc(base(file.source_path)) + "</h2><div class=\"grid\"><span>Media</span><strong>" + esc(file.media_type) + "</strong><span>Folder</span><strong>" + esc(file.folder || "-") + "</strong><span>Final</span><strong>" + esc(file.final_file_name || "-") + "</strong><span>Shot</span><strong>" + esc(shot(file.shot_at)) + "</strong><span>Scene</span><strong>" + esc(source.content?.scene_summary || source.content?.notes || "-") + "</strong><span>Audio</span><strong>" + esc(source.content?.audio_summary || source.content?.audio_transcript || "-") + "</strong><span>Location</span><strong>" + esc(source.location?.label || source.content?.location_guess || "-") + "</strong><span>Source</span><strong>" + esc(file.source_path) + "</strong><span>Export</span><strong>" + esc(file.export_path || "-") + "</strong><span>Tags</span><strong>" + esc((file.tags || []).join(", ")) + "</strong></div>";
    }
    function render() { stats(); renderMaps(); renderRows(); renderDetail(); }
    $("filter").addEventListener("input", renderRows);
    $("clearFilter").addEventListener("click", () => { $("filter").value = ""; selectedFolder = ""; selectedTag = ""; render(); });
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
