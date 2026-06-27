package media

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

const (
	serviceName = "Clip Atlas"
	cliName     = "clip-indexer"
)

var (
	version = "0.1.0-dev"
	commit  = "unknown"
	date    = "unknown"
)

func Run(args []string, stdout, stderr io.Writer) error {
	envWarnings := loadEnvFiles(".env.local", ".env")
	if len(args) > 0 {
		switch args[0] {
		case "dev":
			return runDev(args[1:], stdout, stderr)
		case "export":
			return runExport(args[1:], stdout, stderr, envWarnings)
		case "serve":
			return runServe(args[1:], stdout, stderr, envWarnings)
		case "review":
			return runReview(args[1:], stdout, stderr, envWarnings)
		case "index":
			return runIndex(args[1:], stdout, stderr, envWarnings)
		case "help", "-h", "--help":
			printRootUsage(stderr)
			return nil
		case "--version", "-version":
			fmt.Fprintln(stdout, versionString())
			return nil
		}
	}
	return runIndex(args, stdout, stderr, envWarnings)
}

func printRootUsage(stderr io.Writer) {
	fmt.Fprintf(stderr, "Usage: %s <command> [flags] <media-file-or-directory>...\n\n", cliName)
	fmt.Fprintln(stderr, "Commands:")
	fmt.Fprintln(stderr, "  index    emit a JSON report (default)")
	fmt.Fprintln(stderr, "  export   write a static HTML report bundle")
	fmt.Fprintln(stderr, "  review   write a dry-run review bundle with Mermaid and folder plans")
	fmt.Fprintln(stderr, "  serve    launch the local file-manager web UI")
	fmt.Fprintln(stderr, "  dev      run the web UI and restart it when source files change")
	fmt.Fprintln(stderr, "\nExamples:")
	fmt.Fprintf(stderr, "  %s --pretty --trip seoul ~/Movies/trip\n", cliName)
	fmt.Fprintf(stderr, "  %s export --trip seoul ~/Movies/trip\n", cliName)
	fmt.Fprintf(stderr, "  %s review --trip seoul --dest-root ~/Movies/organized ~/Movies/trip\n", cliName)
	fmt.Fprintf(stderr, "  %s serve --trip seoul ~/Movies/trip\n", cliName)
	fmt.Fprintf(stderr, "  %s dev --trip seoul ~/Movies/trip\n", cliName)
}

func runIndex(args []string, stdout, stderr io.Writer, envWarnings []string) error {
	cfg := defaultConfig()
	showVersion := false

	fs := flag.NewFlagSet(cliName+" index", flag.ContinueOnError)
	fs.SetOutput(stderr)
	addIndexFlags(fs, &cfg)
	fs.BoolVar(&showVersion, "version", false, "print version")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: %s [flags] <media-file-or-directory>...\n\n", cliName)
		fmt.Fprintf(stderr, "       %s index [flags] <media-file-or-directory>...\n\n", cliName)
		fmt.Fprintln(stderr, "Indexes local media and emits JSON with shot dates, tags, and suggested filenames.")
		fmt.Fprintln(stderr, "\nFlags:")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if showVersion {
		fmt.Fprintln(stdout, versionString())
		return nil
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return errors.New("at least one file or directory is required")
	}

	ctx := context.Background()
	report, err := BuildReport(ctx, cfg, fs.Args())
	if err != nil {
		return err
	}
	report.Warnings = append(envWarnings, report.Warnings...)
	refreshReportDerived(&report, reportFilesDiscovered(report))

	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	if cfg.Pretty {
		encoder.SetIndent("", "  ")
	}
	return encoder.Encode(report)
}

func addIndexFlags(fs *flag.FlagSet, cfg *Config) {
	fs.BoolVar(&cfg.Recursive, "recursive", cfg.Recursive, "scan directories recursively")
	fs.BoolVar(&cfg.Recursive, "r", cfg.Recursive, "scan directories recursively")
	fs.BoolVar(&cfg.Pretty, "pretty", cfg.Pretty, "pretty-print JSON output")
	fs.BoolVar(&cfg.IncludeUnsupported, "include-unsupported", cfg.IncludeUnsupported, "include files even when their extension is not a known media type")
	fs.StringVar(&cfg.Trip, "trip", cfg.Trip, "trip or project name to include in suggested filenames")
	fs.StringVar(&cfg.FFProbePath, "ffprobe", cfg.FFProbePath, "path to ffprobe executable")
	fs.StringVar(&cfg.FFMpegPath, "ffmpeg", cfg.FFMpegPath, "path to ffmpeg executable for vision frame extraction")
	fs.StringVar(&cfg.AnalysisLanguage, "analysis-language", cfg.AnalysisLanguage, "analysis output language: auto, ko, en, zh, or ja")
	fs.BoolVar(&cfg.UseLLM, "llm", cfg.UseLLM, "enrich tags and final filenames with an OpenAI-compatible chat endpoint")
	fs.BoolVar(&cfg.UseLLMVision, "llm-vision", cfg.UseLLMVision, "sample visual media frames and ask the LLM for scene and location hints")
	fs.BoolVar(&cfg.UseLLMAudio, "llm-audio", cfg.UseLLMAudio, "extract audio and ask the LLM for transcript, sound tags, and spoken context")
	fs.BoolVar(&cfg.VisionAdaptive, "vision-adaptive", cfg.VisionAdaptive, "adapt vision frame sampling to clip duration; disable to use --vision-frames exactly")
	fs.IntVar(&cfg.VisionFrames, "vision-frames", cfg.VisionFrames, "minimum frames to sample per visual media file when adaptive vision is enabled; exact count when --vision-adaptive=false")
	fs.IntVar(&cfg.VisionSampleIntervalSeconds, "vision-sample-interval", cfg.VisionSampleIntervalSeconds, "sample one vision frame about every N seconds; 0 uses adaptive duration sampling or --vision-frames")
	fs.IntVar(&cfg.VisionMaxItems, "vision-max-items", cfg.VisionMaxItems, "maximum visual media files to analyze with --llm-vision; 0 means all")
	fs.StringVar(&cfg.VisionPromptFile, "vision-prompt-file", cfg.VisionPromptFile, "path to a custom system prompt for --llm-vision")
	fs.IntVar(&cfg.AudioMaxSeconds, "audio-max-seconds", cfg.AudioMaxSeconds, "maximum seconds of audio to transcribe per media file when --llm-audio is enabled")
	fs.IntVar(&cfg.AudioMaxItems, "audio-max-items", cfg.AudioMaxItems, "maximum media files to analyze with --llm-audio; 0 means all")
	fs.StringVar(&cfg.AudioModel, "audio-model", cfg.AudioModel, "audio transcription model name")
	fs.StringVar(&cfg.LLMBaseURL, "llm-base-url", cfg.LLMBaseURL, "LLM API base URL")
	fs.Func("llm-api-key", "LLM API key", func(value string) error {
		cfg.LLMAPIKey = value
		return nil
	})
	fs.StringVar(&cfg.LLMModel, "llm-model", cfg.LLMModel, "LLM model name")
	fs.IntVar(&cfg.LLMTimeoutSeconds, "llm-timeout", cfg.LLMTimeoutSeconds, "LLM request timeout in seconds")
}

func BuildReport(ctx context.Context, cfg Config, inputs []string) (Report, error) {
	if err := validateConfig(cfg); err != nil {
		return Report{}, err
	}

	paths, discoveryWarnings, err := Discover(inputs, cfg.Recursive, cfg.IncludeUnsupported)
	if err != nil {
		return Report{}, err
	}

	report := Report{
		Service: ServiceInfo{
			Name:    serviceName,
			CLI:     cliName,
			Version: version,
		},
		GeneratedAt: time.Now().Format(time.RFC3339),
		Options: ReportOptions{
			Recursive:                   cfg.Recursive,
			Trip:                        cfg.Trip,
			AnalysisLanguage:            normalizeAnalysisLanguage(cfg.AnalysisLanguage),
			LLM:                         cfg.UseLLM,
			LLMVision:                   cfg.UseLLMVision,
			LLMAudio:                    cfg.UseLLMAudio,
			AutoAnalyze:                 cfg.AutoAnalyze,
			AutoAnalyzeMaxItems:         cfg.AutoAnalyzeMaxItems,
			VisionAdaptive:              cfg.VisionAdaptive,
			VisionFrames:                cfg.VisionFrames,
			VisionSampleIntervalSeconds: cfg.VisionSampleIntervalSeconds,
			VisionMaxItems:              cfg.VisionMaxItems,
			VisionPromptFile:            cfg.VisionPromptFile,
			AudioMaxSeconds:             cfg.AudioMaxSeconds,
			AudioMaxItems:               cfg.AudioMaxItems,
		},
		Warnings: append([]string{}, discoveryWarnings...),
	}

	for index, path := range paths {
		probe := Probe(ctx, cfg.FFProbePath, path)
		item := BuildItem(path, index+1, cfg.Trip, probe)
		report.Warnings = append(report.Warnings, applyAnalysisCache(&item)...)
		updateItemGroup(&item)
		report.Items = append(report.Items, item)
	}

	if cfg.UseLLM && len(report.Items) > 0 {
		llmCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.LLMTimeoutSeconds)*time.Second)
		defer cancel()
		if warnings := EnrichWithLLM(llmCtx, cfg, report.Items); len(warnings) > 0 {
			report.Warnings = append(report.Warnings, warnings...)
		}
	}
	if cfg.UseLLMVision && len(report.Items) > 0 {
		visionTimeout := time.Duration(max(1, cfg.LLMTimeoutSeconds*max(1, visionItemCount(cfg, len(report.Items))))) * time.Second
		visionCtx, cancel := context.WithTimeout(ctx, visionTimeout)
		defer cancel()
		if warnings := EnrichWithVision(visionCtx, cfg, report.Items); len(warnings) > 0 {
			report.Warnings = append(report.Warnings, warnings...)
		}
	}
	if cfg.UseLLMAudio && len(report.Items) > 0 {
		audioTimeout := time.Duration(max(1, cfg.LLMTimeoutSeconds*max(1, analysisItemCount(cfg.AudioMaxItems, len(report.Items))))) * time.Second
		audioCtx, cancel := context.WithTimeout(ctx, audioTimeout)
		defer cancel()
		if warnings := EnrichWithAudio(audioCtx, cfg, report.Items); len(warnings) > 0 {
			report.Warnings = append(report.Warnings, warnings...)
		}
	}
	if cfg.UseLLM || cfg.UseLLMVision || cfg.UseLLMAudio {
		updateItemGroups(report.Items)
		report.Warnings = append(report.Warnings, saveAnalysisCaches(report.Items)...)
	}

	refreshReportDerived(&report, len(paths))
	return report, nil
}

func validateConfig(cfg Config) error {
	if normalizeAnalysisLanguage(cfg.AnalysisLanguage) == "" {
		return errors.New("--analysis-language must be one of auto, ko, en, zh, ja")
	}
	if cfg.UseLLMVision || cfg.UseLLMAudio {
		cfg.UseLLM = true
	}
	if cfg.UseLLMVision {
		if cfg.VisionFrames < 1 {
			return errors.New("--vision-frames must be at least 1")
		}
		if cfg.VisionMaxItems < 0 {
			return errors.New("--vision-max-items must be 0 or greater")
		}
		if cfg.VisionSampleIntervalSeconds < 0 {
			return errors.New("--vision-sample-interval must be 0 or greater")
		}
		if strings.TrimSpace(cfg.VisionPromptFile) != "" {
			if _, err := os.Stat(cfg.VisionPromptFile); err != nil {
				return fmt.Errorf("--vision-prompt-file is not readable: %w", err)
			}
		}
	}
	if cfg.UseLLMAudio {
		if cfg.AudioMaxSeconds < 1 {
			return errors.New("--audio-max-seconds must be at least 1")
		}
		if cfg.AudioMaxItems < 0 {
			return errors.New("--audio-max-items must be 0 or greater")
		}
		if strings.TrimSpace(cfg.AudioModel) == "" {
			return errors.New("--llm-audio requires --audio-model or LLM_AUDIO_MODEL/OPENAI_AUDIO_MODEL")
		}
	}
	if cfg.UseLLM {
		if strings.TrimSpace(cfg.LLMModel) == "" {
			return errors.New("--llm or --llm-vision requires --llm-model or LLM_MODEL/OPENAI_MODEL")
		}
		if isOpenAIHosted(cfg.LLMBaseURL) && strings.TrimSpace(cfg.LLMAPIKey) == "" {
			return errors.New("--llm or --llm-vision with the default OpenAI-compatible base URL requires --llm-api-key or LLM_API_KEY/OPENAI_API_KEY")
		}
	}
	return nil
}

func normalizeAnalysisLanguage(language string) string {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "", "auto", "default":
		return "auto"
	case "ko", "kr", "korean", "한국어":
		return "ko"
	case "en", "eng", "english":
		return "en"
	case "zh", "cn", "chinese", "中文", "중국어":
		return "zh"
	case "ja", "jp", "japanese", "日本語", "일본어":
		return "ja"
	default:
		return ""
	}
}

func analysisLanguageInstruction(cfg Config) string {
	switch normalizeAnalysisLanguage(cfg.AnalysisLanguage) {
	case "ko":
		return "Write scene_summary, audio_summary, notes, suggested_slug, final_file_name, and natural-language tags in Korean when possible. Keep proper nouns and place names in their original script when that is clearer."
	case "en":
		return "Write scene_summary, audio_summary, notes, suggested_slug, final_file_name, and natural-language tags in English."
	case "zh":
		return "Write scene_summary, audio_summary, notes, suggested_slug, final_file_name, and natural-language tags in Chinese when possible. Keep proper nouns and place names in their original script when that is clearer."
	case "ja":
		return "Write scene_summary, audio_summary, notes, suggested_slug, final_file_name, and natural-language tags in Japanese when possible. Keep proper nouns and place names in their original script when that is clearer."
	default:
		return "Choose the output language from the clip context and existing metadata; preserve meaningful Korean, English, Chinese, and Japanese words instead of forcing translation."
	}
}

func versionString() string {
	if commit == "unknown" && date == "unknown" {
		return fmt.Sprintf("%s %s", cliName, version)
	}
	return fmt.Sprintf("%s %s (commit %s, built %s)", cliName, version, commit, date)
}

func visionItemCount(cfg Config, itemCount int) int {
	return analysisItemCount(cfg.VisionMaxItems, itemCount)
}

func analysisItemCount(limit int, itemCount int) int {
	if limit > 0 && limit < itemCount {
		return limit
	}
	return itemCount
}

func defaultConfig() Config {
	return Config{
		FFProbePath:                 envOr("FFPROBE_PATH", "ffprobe"),
		FFMpegPath:                  envOr("FFMPEG_PATH", "ffmpeg"),
		Recursive:                   envBoolOr("CLIP_INDEXER_RECURSIVE", true),
		Host:                        envOr("CLIP_INDEXER_HOST", "127.0.0.1"),
		Port:                        envIntOr("CLIP_INDEXER_PORT", 4317),
		AnalysisLanguage:            envOr("CLIP_INDEXER_ANALYSIS_LANGUAGE", "auto"),
		AutoAnalyze:                 envBoolOr("CLIP_INDEXER_AUTO_ANALYZE", false),
		AutoAnalyzeMaxItems:         envIntOr("CLIP_INDEXER_AUTO_ANALYZE_MAX_ITEMS", 3),
		VisionAdaptive:              envBoolOr("CLIP_INDEXER_VISION_ADAPTIVE", true),
		VisionFrames:                envIntOr("CLIP_INDEXER_VISION_FRAMES", 2),
		VisionSampleIntervalSeconds: envIntOr("CLIP_INDEXER_VISION_SAMPLE_INTERVAL", 0),
		VisionMaxItems:              envIntOr("CLIP_INDEXER_VISION_MAX_ITEMS", 12),
		VisionPromptFile:            envOr("CLIP_INDEXER_VISION_PROMPT_FILE", ""),
		AudioMaxSeconds:             envIntOr("CLIP_INDEXER_AUDIO_MAX_SECONDS", 45),
		AudioMaxItems:               envIntOr("CLIP_INDEXER_AUDIO_MAX_ITEMS", 12),
		AudioModel:                  envOrAny("whisper-1", "LLM_AUDIO_MODEL", "OPENAI_AUDIO_MODEL"),
		LLMBaseURL:                  envOrAny("https://api.openai.com/v1", "LLM_BASE_URL", "OPENAI_BASE_URL"),
		LLMAPIKey:                   envOrAny("", "LLM_API_KEY", "OPENAI_API_KEY"),
		LLMModel:                    envOrAny("", "LLM_MODEL", "OPENAI_MODEL"),
		LLMTimeoutSeconds:           30,
	}
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envIntOr(key string, fallback int) int {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		var parsed int
		if _, err := fmt.Sscanf(value, "%d", &parsed); err == nil {
			return parsed
		}
	}
	return fallback
}

func envBoolOr(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch value {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func envOrAny(fallback string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return fallback
}

func isOpenAIHosted(baseURL string) bool {
	normalized := strings.ToLower(strings.TrimSpace(baseURL))
	return normalized == "" || strings.Contains(normalized, "api.openai.com")
}

func supportsAudioTranscriptions(baseURL string) bool {
	normalized := strings.ToLower(strings.TrimSpace(baseURL))
	return !strings.Contains(normalized, "generativelanguage.googleapis.com")
}

func summarize(items []Item, discovered int, reportWarnings int) Summary {
	summary := Summary{
		FilesDiscovered: discovered,
		FilesIndexed:    len(items),
		Warnings:        reportWarnings,
	}
	for _, item := range items {
		mediaType := itemMediaType(item)
		if item.ShotAt != "" {
			summary.WithShotDate++
		}
		if mediaType == mediaTypeImage {
			summary.WithImageFile++
		}
		if mediaType == mediaTypeVideo {
			summary.WithVideoStream++
		}
		if item.Audio != nil {
			summary.WithAudioStream++
		}
		if item.Location != nil {
			summary.WithLocation++
		}
		if item.Content != nil {
			summary.WithContent++
		}
		summary.Warnings += len(item.Warnings)
	}
	return summary
}
