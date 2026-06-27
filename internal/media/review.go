package media

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type reviewOptions struct {
	OutputDir        string
	DestinationRoot  string
	FolderDepth      int
	UseLLMFolderPlan bool
}

type reviewBundle struct {
	Service           ServiceInfo        `json:"service"`
	GeneratedAt       string             `json:"generated_at"`
	DryRun            bool               `json:"dry_run"`
	Trip              string             `json:"trip,omitempty"`
	Inputs            []string           `json:"inputs"`
	OutputDir         string             `json:"output_dir"`
	DestinationRoot   string             `json:"destination_root"`
	UsedLLMFolderPlan bool               `json:"used_llm_folder_plan"`
	ReportPath        string             `json:"report_path"`
	MindmapPath       string             `json:"mindmap_path"`
	FolderPlanPath    string             `json:"folder_plan_path"`
	RenamePlanPath    string             `json:"rename_plan_path"`
	ApplyRequestPath  string             `json:"apply_request_path"`
	SummaryPath       string             `json:"summary_path"`
	ReviewPath        string             `json:"review_path"`
	Summary           Summary            `json:"summary"`
	ExistingFolders   []folderEntry      `json:"existing_folders,omitempty"`
	Folders           []plannedFolder    `json:"folders"`
	Assignments       []folderAssignment `json:"assignments"`
	Items             []reviewPlanItem   `json:"items"`
	Warnings          []string           `json:"warnings,omitempty"`
	ConflictCount     int                `json:"conflict_count"`
	TargetExistsCount int                `json:"target_exists_count"`
	RenameCount       int                `json:"rename_count"`
	MoveCount         int                `json:"move_count"`
}

type reviewPlanItem struct {
	SourcePath       string     `json:"source_path"`
	TargetPath       string     `json:"target_path"`
	OriginalFileName string     `json:"original_file_name"`
	FinalFileName    string     `json:"final_file_name"`
	Folder           string     `json:"folder"`
	ShotAt           string     `json:"shot_at,omitempty"`
	Tags             []string   `json:"tags,omitempty"`
	Group            *GroupInfo `json:"group,omitempty"`
	SceneSummary     string     `json:"scene_summary,omitempty"`
	Location         string     `json:"location,omitempty"`
	Rename           bool       `json:"rename"`
	Move             bool       `json:"move"`
	Conflict         bool       `json:"conflict"`
	ConflictReason   string     `json:"conflict_reason,omitempty"`
	Reason           string     `json:"reason,omitempty"`
}

type reviewGroupSummary struct {
	Key    string
	Label  string
	Count  int
	Tags   []reviewCount
	Places []reviewCount
	Items  []Item
}

type reviewCount struct {
	Label string
	Count int
}

func runReview(args []string, stdout, stderr io.Writer, envWarnings []string) error {
	cfg := defaultConfig()
	options := reviewOptions{}

	fs := flag.NewFlagSet(cliName+" review", flag.ContinueOnError)
	fs.SetOutput(stderr)
	addIndexFlags(fs, &cfg)
	fs.StringVar(&options.OutputDir, "out-dir", "", "directory for the review bundle; default tmp/clip-atlas-review/<timestamp>")
	fs.StringVar(&options.DestinationRoot, "dest-root", "", "destination root to preview organized files; default <out-dir>/organized")
	fs.IntVar(&options.FolderDepth, "folder-depth", 0, "maximum depth for existing destination folders; 0 means unlimited")
	fs.BoolVar(&options.UseLLMFolderPlan, "llm-folder-plan", false, "use LLM folder planning when credentials are configured")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: %s review [flags] <video-file-or-directory>...\n\n", cliName)
		fmt.Fprintln(stderr, "Writes a dry-run bundle with report JSON, Mermaid mindmap, folder plan, rename CSV, and apply request JSON.")
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
	if options.FolderDepth < 0 {
		return errors.New("--folder-depth must be 0 or greater")
	}

	ctx := context.Background()
	report, err := BuildReport(ctx, cfg, fs.Args())
	if err != nil {
		return err
	}
	report.Warnings = append(envWarnings, report.Warnings...)
	report.Summary = summarize(report.Items, report.Summary.FilesDiscovered, len(report.Warnings))

	bundle, applyRequest, err := buildReviewBundle(ctx, cfg, report, fs.Args(), options)
	if err != nil {
		return err
	}
	if err := writeReviewBundle(bundle, report, applyRequest); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "Review bundle written to %s\n", bundle.OutputDir)
	fmt.Fprintf(stdout, "Summary: %s\n", bundle.SummaryPath)
	fmt.Fprintf(stdout, "Mermaid: %s\n", bundle.MindmapPath)
	fmt.Fprintf(stdout, "Folder plan: %s\n", bundle.FolderPlanPath)
	if bundle.ConflictCount > 0 || bundle.TargetExistsCount > 0 {
		fmt.Fprintf(stdout, "Warnings: %d conflict(s), %d existing target(s)\n", bundle.ConflictCount, bundle.TargetExistsCount)
	}
	return nil
}

func buildReviewBundle(ctx context.Context, cfg Config, report Report, inputs []string, options reviewOptions) (reviewBundle, applyRequest, error) {
	outputDir := strings.TrimSpace(options.OutputDir)
	if outputDir == "" {
		outputDir = filepath.Join("tmp", "clip-atlas-review", time.Now().Format("20060102-150405"))
	}
	absOutputDir, err := filepath.Abs(outputDir)
	if err != nil {
		return reviewBundle{}, applyRequest{}, err
	}

	destinationRoot := strings.TrimSpace(options.DestinationRoot)
	if destinationRoot == "" {
		destinationRoot = filepath.Join(absOutputDir, "organized")
	}
	absDestinationRoot, err := filepath.Abs(destinationRoot)
	if err != nil {
		return reviewBundle{}, applyRequest{}, err
	}

	warnings := append([]string{}, report.Warnings...)
	existingFolders, folderWarnings := existingFoldersForReview(absDestinationRoot, options.FolderDepth)
	warnings = append(warnings, folderWarnings...)

	plan, usedLLM, planWarnings := reviewFolderPlan(ctx, cfg, report.Items, existingFolders, options.UseLLMFolderPlan)
	warnings = append(warnings, planWarnings...)

	items, request := reviewItemsAndApplyRequest(report.Items, plan.Assignments, absDestinationRoot)
	renameCount, moveCount, conflictCount, targetExistsCount := countReviewItems(items)
	if conflictCount > 0 {
		warnings = append(warnings, fmt.Sprintf("review has %d duplicate target path conflict(s)", conflictCount))
	}
	if targetExistsCount > 0 {
		warnings = append(warnings, fmt.Sprintf("review has %d target path(s) that already exist", targetExistsCount))
	}

	bundle := reviewBundle{
		Service:           ServiceInfo{Name: serviceName, CLI: cliName, Version: version},
		GeneratedAt:       time.Now().Format(time.RFC3339),
		DryRun:            true,
		Trip:              report.Options.Trip,
		Inputs:            append([]string{}, inputs...),
		OutputDir:         absOutputDir,
		DestinationRoot:   absDestinationRoot,
		UsedLLMFolderPlan: usedLLM,
		ReportPath:        filepath.Join(absOutputDir, "report.json"),
		MindmapPath:       filepath.Join(absOutputDir, "mindmap.mmd"),
		FolderPlanPath:    filepath.Join(absOutputDir, "folder-plan.json"),
		RenamePlanPath:    filepath.Join(absOutputDir, "rename-plan.csv"),
		ApplyRequestPath:  filepath.Join(absOutputDir, "apply-request.json"),
		SummaryPath:       filepath.Join(absOutputDir, "summary.md"),
		ReviewPath:        filepath.Join(absOutputDir, "review.json"),
		Summary:           report.Summary,
		ExistingFolders:   existingFolders,
		Folders:           plan.Folders,
		Assignments:       plan.Assignments,
		Items:             items,
		Warnings:          warnings,
		ConflictCount:     conflictCount,
		TargetExistsCount: targetExistsCount,
		RenameCount:       renameCount,
		MoveCount:         moveCount,
	}
	return bundle, request, nil
}

func existingFoldersForReview(root string, depth int) ([]folderEntry, []string) {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, []string{"destination root does not exist yet; planning against an empty folder tree"}
		}
		return nil, []string{fmt.Sprintf("could not inspect destination root %s: %v", root, err)}
	}
	if !info.IsDir() {
		return nil, []string{fmt.Sprintf("destination root is not a directory: %s", root)}
	}
	folders, warnings, err := listSubfolders(root, depth)
	if err != nil {
		return nil, []string{fmt.Sprintf("could not list destination folders: %v", err)}
	}
	return folders, warnings
}

func reviewFolderPlan(ctx context.Context, cfg Config, items []Item, existingFolders []folderEntry, useLLM bool) (folderPlanOutput, bool, []string) {
	warnings := []string{}
	if useLLM {
		folderCfg := cfg
		folderCfg.UseLLM = true
		if err := validateConfig(folderCfg); err != nil {
			warnings = append(warnings, "folder plan LLM skipped: "+err.Error())
		} else {
			llmCtx, cancel := context.WithTimeout(ctx, time.Duration(max(1, cfg.LLMTimeoutSeconds))*time.Second)
			defer cancel()
			plan, err := planFoldersWithLLM(llmCtx, folderCfg, items, existingFolders)
			if err != nil {
				warnings = append(warnings, "folder plan LLM failed: "+err.Error())
			} else {
				return completeFolderPlan(plan, items, existingFolders), true, warnings
			}
		}
	}
	return deterministicFolderPlan(items, existingFolders), false, warnings
}

func reviewItemsAndApplyRequest(items []Item, assignments []folderAssignment, destinationRoot string) ([]reviewPlanItem, applyRequest) {
	byPath := map[string]Item{}
	for _, item := range items {
		byPath[item.SourcePath] = item
	}

	targetCounts := map[string]int{}
	targetExists := map[string]bool{}
	type pendingPlan struct {
		assignment folderAssignment
		item       Item
		targetPath string
	}
	pending := make([]pendingPlan, 0, len(assignments))
	for _, assignment := range assignments {
		item, ok := byPath[assignment.SourcePath]
		if !ok {
			continue
		}
		targetPath := filepath.Join(destinationRoot, filepath.FromSlash(assignment.Folder), assignment.FinalFileName)
		targetCounts[targetPath]++
		if _, err := os.Stat(targetPath); err == nil && !sameCleanPath(targetPath, item.SourcePath) {
			targetExists[targetPath] = true
		}
		pending = append(pending, pendingPlan{assignment: assignment, item: item, targetPath: targetPath})
	}

	plans := make([]reviewPlanItem, 0, len(pending))
	request := applyRequest{Operations: make([]applyOperation, 0, len(pending))}
	for _, next := range pending {
		item := next.item
		assignment := next.assignment
		targetPath := next.targetPath
		targetDir := filepath.Dir(targetPath)
		sourceDir := filepath.Dir(item.SourcePath)
		conflictReason := ""
		if targetCounts[targetPath] > 1 {
			conflictReason = "duplicate target path"
		}
		if targetExists[targetPath] {
			if conflictReason != "" {
				conflictReason += "; "
			}
			conflictReason += "target already exists"
		}

		location := ""
		if item.Location != nil {
			location = strings.TrimSpace(strings.Join([]string{item.Location.Label, item.Location.Notes}, " "))
		}
		sceneSummary := ""
		if item.Content != nil {
			sceneSummary = item.Content.SceneSummary
			if location == "" {
				location = item.Content.LocationGuess
			}
		}

		plans = append(plans, reviewPlanItem{
			SourcePath:       item.SourcePath,
			TargetPath:       targetPath,
			OriginalFileName: item.OriginalFileName,
			FinalFileName:    assignment.FinalFileName,
			Folder:           assignment.Folder,
			ShotAt:           item.ShotAt,
			Tags:             append([]string{}, item.Tags...),
			Group:            item.Group,
			SceneSummary:     sceneSummary,
			Location:         strings.TrimSpace(location),
			Rename:           filepath.Base(item.SourcePath) != assignment.FinalFileName,
			Move:             !sameCleanPath(sourceDir, targetDir),
			Conflict:         conflictReason != "",
			ConflictReason:   conflictReason,
			Reason:           assignment.Reason,
		})

		if conflictReason != "" {
			continue
		}
		request.Operations = append(request.Operations, applyOperation{
			SourcePath:   item.SourcePath,
			FinalName:    assignment.FinalFileName,
			Tags:         append([]string{}, item.Tags...),
			Rename:       true,
			MoveToGroup:  true,
			GroupRoot:    destinationRoot,
			TargetFolder: assignment.Folder,
			WriteSidecar: true,
		})
	}
	return plans, request
}

func countReviewItems(items []reviewPlanItem) (renameCount int, moveCount int, conflictCount int, targetExistsCount int) {
	for _, item := range items {
		if item.Rename {
			renameCount++
		}
		if item.Move {
			moveCount++
		}
		if strings.Contains(item.ConflictReason, "duplicate target path") {
			conflictCount++
		}
		if strings.Contains(item.ConflictReason, "target already exists") {
			targetExistsCount++
		}
	}
	return renameCount, moveCount, conflictCount, targetExistsCount
}

func writeReviewBundle(bundle reviewBundle, report Report, request applyRequest) error {
	if err := os.MkdirAll(bundle.OutputDir, 0o755); err != nil {
		return err
	}
	if err := writePrettyJSONFile(bundle.ReportPath, report); err != nil {
		return err
	}
	if err := os.WriteFile(bundle.MindmapPath, []byte(buildMermaidMindmap(report.Items)), 0o644); err != nil {
		return err
	}
	if err := writePrettyJSONFile(bundle.FolderPlanPath, folderPlanOutput{Folders: bundle.Folders, Assignments: bundle.Assignments}); err != nil {
		return err
	}
	if err := writeReviewCSV(bundle.RenamePlanPath, bundle.Items); err != nil {
		return err
	}
	if err := writePrettyJSONFile(bundle.ApplyRequestPath, request); err != nil {
		return err
	}
	if err := os.WriteFile(bundle.SummaryPath, []byte(renderReviewSummary(bundle)), 0o644); err != nil {
		return err
	}
	if err := writePrettyJSONFile(bundle.ReviewPath, bundle); err != nil {
		return err
	}
	return nil
}

func writePrettyJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func writeReviewCSV(path string, items []reviewPlanItem) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()
	if err := writer.Write([]string{
		"source_path",
		"target_path",
		"original_file_name",
		"final_file_name",
		"folder",
		"rename",
		"move",
		"conflict",
		"conflict_reason",
		"reason",
		"tags",
	}); err != nil {
		return err
	}
	for _, item := range items {
		if err := writer.Write([]string{
			item.SourcePath,
			item.TargetPath,
			item.OriginalFileName,
			item.FinalFileName,
			item.Folder,
			strconv.FormatBool(item.Rename),
			strconv.FormatBool(item.Move),
			strconv.FormatBool(item.Conflict),
			item.ConflictReason,
			item.Reason,
			strings.Join(item.Tags, " "),
		}); err != nil {
			return err
		}
	}
	return writer.Error()
}

func renderReviewSummary(bundle reviewBundle) string {
	var builder strings.Builder
	fmt.Fprintln(&builder, "# Clip Atlas Review")
	fmt.Fprintln(&builder)
	fmt.Fprintf(&builder, "- Generated: %s\n", bundle.GeneratedAt)
	fmt.Fprintf(&builder, "- Dry run: %t\n", bundle.DryRun)
	fmt.Fprintf(&builder, "- Destination root: `%s`\n", bundle.DestinationRoot)
	fmt.Fprintf(&builder, "- Files indexed: %d\n", bundle.Summary.FilesIndexed)
	fmt.Fprintf(&builder, "- With content: %d\n", bundle.Summary.WithContent)
	fmt.Fprintf(&builder, "- Folder planner: %s\n", reviewPlannerLabel(bundle.UsedLLMFolderPlan))
	fmt.Fprintf(&builder, "- Planned folders: %d\n", len(bundle.Folders))
	fmt.Fprintf(&builder, "- Planned moves: %d\n", bundle.MoveCount)
	fmt.Fprintf(&builder, "- Planned renames: %d\n", bundle.RenameCount)
	fmt.Fprintf(&builder, "- Conflicts: %d\n", bundle.ConflictCount+bundle.TargetExistsCount)
	fmt.Fprintln(&builder)

	fmt.Fprintln(&builder, "## Output Files")
	fmt.Fprintf(&builder, "- `report.json`: full analysis report\n")
	fmt.Fprintf(&builder, "- `mindmap.mmd`: Mermaid mind map\n")
	fmt.Fprintf(&builder, "- `folder-plan.json`: planned folders and assignments\n")
	fmt.Fprintf(&builder, "- `rename-plan.csv`: spreadsheet-friendly rename/move preview\n")
	fmt.Fprintf(&builder, "- `apply-request.json`: dry-run apply payload for the web/API flow\n")
	fmt.Fprintf(&builder, "- `review.json`: combined review metadata\n")
	fmt.Fprintln(&builder)

	if len(bundle.Warnings) > 0 {
		fmt.Fprintln(&builder, "## Warnings")
		for _, warning := range bundle.Warnings {
			fmt.Fprintf(&builder, "- %s\n", warning)
		}
		fmt.Fprintln(&builder)
	}

	fmt.Fprintln(&builder, "## Folder Plan")
	fmt.Fprintln(&builder)
	fmt.Fprintln(&builder, "| Folder | Count | Existing | Reason |")
	fmt.Fprintln(&builder, "| --- | ---: | --- | --- |")
	for _, folder := range bundle.Folders {
		fmt.Fprintf(&builder, "| `%s` | %d | %t | %s |\n", escapeMarkdownCell(folder.Folder), folder.Count, folder.Existing, escapeMarkdownCell(folder.Reason))
	}
	fmt.Fprintln(&builder)

	fmt.Fprintln(&builder, "## Rename And Move Preview")
	fmt.Fprintln(&builder)
	fmt.Fprintln(&builder, "| Source | Folder | Final name | Status |")
	fmt.Fprintln(&builder, "| --- | --- | --- | --- |")
	for _, item := range bundle.Items {
		status := "ok"
		if item.Conflict {
			status = item.ConflictReason
		}
		fmt.Fprintf(&builder, "| `%s` | `%s` | `%s` | %s |\n",
			escapeMarkdownCell(item.OriginalFileName),
			escapeMarkdownCell(item.Folder),
			escapeMarkdownCell(item.FinalFileName),
			escapeMarkdownCell(status),
		)
	}
	fmt.Fprintln(&builder)

	fmt.Fprintln(&builder, "## 처리 흐름")
	fmt.Fprintln(&builder)
	fmt.Fprintln(&builder, "이 번들은 실제 파일을 이동하지 않는 dry-run 결과입니다. `summary.md`와 `rename-plan.csv`에서 이름과 폴더를 확인한 뒤, 같은 입력과 destination root로 웹 UI를 열어 적용하면 됩니다.")
	fmt.Fprintln(&builder)
	fmt.Fprintln(&builder, "```bash")
	fmt.Fprintf(&builder, "go run ./cmd/clip-indexer serve --trip %q %s\n", firstInputLabel(bundle), strings.Join(shellQuoteArgs(bundle.Inputs), " "))
	fmt.Fprintln(&builder, "```")
	return builder.String()
}

func reviewPlannerLabel(usedLLM bool) string {
	if usedLLM {
		return "LLM"
	}
	return "deterministic"
}

func buildMermaidMindmap(items []Item) string {
	lines := []string{"mindmap", fmt.Sprintf("  root((Clip Atlas %d))", len(items))}
	if len(items) == 0 {
		return strings.Join(lines, "\n") + "\n"
	}
	for _, group := range reviewGroups(items, 14) {
		lines = append(lines, "    "+mermaidLabel(fmt.Sprintf("%s (%d)", group.Label, group.Count)))
		if len(group.Tags) > 0 {
			lines = append(lines, "      Tags")
			for _, tag := range firstReviewCounts(group.Tags, 10) {
				lines = append(lines, "        "+mermaidLabel(fmt.Sprintf("%s %d", tag.Label, tag.Count)))
			}
		}
		if len(group.Places) > 0 {
			lines = append(lines, "      Places")
			for _, place := range firstReviewCounts(group.Places, 6) {
				lines = append(lines, "        "+mermaidLabel(fmt.Sprintf("%s %d", place.Label, place.Count)))
			}
		}
		if len(group.Items) > 0 {
			lines = append(lines, "      Clips")
			for _, item := range firstReviewItems(group.Items, 5) {
				lines = append(lines, "        "+mermaidLabel(filepath.Base(item.SourcePath)))
			}
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func reviewGroups(items []Item, limit int) []reviewGroupSummary {
	groupMap := map[string]*reviewGroupSummary{}
	tagCounts := map[string]map[string]int{}
	placeCounts := map[string]map[string]int{}
	for _, item := range items {
		key := "other"
		label := "Other"
		if item.Group != nil {
			if item.Group.Key != "" {
				key = item.Group.Key
			} else if item.Group.Folder != "" {
				key = item.Group.Folder
			}
			if item.Group.Label != "" {
				label = item.Group.Label
			}
		}
		group := groupMap[key]
		if group == nil {
			group = &reviewGroupSummary{Key: key, Label: label}
			groupMap[key] = group
			tagCounts[key] = map[string]int{}
			placeCounts[key] = map[string]int{}
		}
		group.Count++
		group.Items = append(group.Items, item)
		for _, tag := range item.Tags {
			if tag != "" {
				tagCounts[key][tag]++
			}
		}
		for _, place := range itemPlaces(item) {
			placeCounts[key][place]++
		}
	}

	groups := make([]reviewGroupSummary, 0, len(groupMap))
	for key, group := range groupMap {
		group.Tags = sortedReviewCounts(tagCounts[key])
		group.Places = sortedReviewCounts(placeCounts[key])
		sort.Slice(group.Items, func(i, j int) bool {
			return group.Items[i].SourcePath < group.Items[j].SourcePath
		})
		groups = append(groups, *group)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Count == groups[j].Count {
			return groups[i].Label < groups[j].Label
		}
		return groups[i].Count > groups[j].Count
	})
	if limit > 0 && len(groups) > limit {
		return groups[:limit]
	}
	return groups
}

func itemPlaces(item Item) []string {
	seen := map[string]bool{}
	places := []string{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		places = append(places, value)
	}
	if item.Location != nil {
		add(item.Location.Label)
	}
	if item.Content != nil {
		add(item.Content.LocationGuess)
	}
	return places
}

func sortedReviewCounts(counts map[string]int) []reviewCount {
	values := make([]reviewCount, 0, len(counts))
	for label, count := range counts {
		values = append(values, reviewCount{Label: label, Count: count})
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].Count == values[j].Count {
			return values[i].Label < values[j].Label
		}
		return values[i].Count > values[j].Count
	})
	return values
}

func firstReviewCounts(values []reviewCount, limit int) []reviewCount {
	if limit > 0 && len(values) > limit {
		return values[:limit]
	}
	return values
}

func firstReviewItems(values []Item, limit int) []Item {
	if limit > 0 && len(values) > limit {
		return values[:limit]
	}
	return values
}

func mermaidLabel(value string) string {
	value = strings.Map(func(r rune) rune {
		switch r {
		case '\r', '\n', '\t':
			return ' '
		case '(', ')', '{', '}', '[', ']', '<', '>', '"':
			return -1
		default:
			return r
		}
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	value = truncateRunes(strings.TrimSpace(value), 80)
	if value == "" {
		return "untitled"
	}
	return value
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func escapeMarkdownCell(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	return value
}

func sameCleanPath(left string, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr == nil {
		left = leftAbs
	}
	if rightErr == nil {
		right = rightAbs
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

func firstInputLabel(bundle reviewBundle) string {
	if strings.TrimSpace(bundle.Trip) != "" {
		return bundle.Trip
	}
	if len(bundle.Inputs) == 0 {
		return "Clip Atlas Review"
	}
	return filepath.Base(bundle.Inputs[0])
}

func shellQuoteArgs(args []string) []string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "" {
			quoted = append(quoted, "''")
			continue
		}
		if !strings.ContainsAny(arg, " \t\n'\"\\$`!*?[]{}()<>|&;") {
			quoted = append(quoted, arg)
			continue
		}
		quoted = append(quoted, "'"+strings.ReplaceAll(arg, "'", "'\\''")+"'")
	}
	return quoted
}
