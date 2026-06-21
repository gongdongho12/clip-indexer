package media

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

var filenameDatePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(20\d{2})([01]\d)([0-3]\d)[-_ ]?([0-2]\d)([0-5]\d)([0-5]\d)`),
	regexp.MustCompile(`(?i)(20\d{2})[-_]([01]\d)[-_]([0-3]\d)[-_ ]+([0-2]\d)[-_]?([0-5]\d)[-_]?([0-5]\d)`),
	regexp.MustCompile(`(?i)(20\d{2})([01]\d)([0-3]\d)`),
}

func BuildItem(path string, sequence int, trip string, probe ProbeResult) Item {
	info, statErr := os.Stat(path)
	originalName := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(path))
	item := Item{
		SourcePath:       path,
		OriginalFileName: originalName,
		Extension:        ext,
		FormatName:       probe.FormatName,
		DurationSeconds:  round(probe.DurationSeconds, 3),
		Video:            probe.Video,
		Audio:            probe.Audio,
		Location:         probe.Location,
		Warnings:         append([]string{}, probe.Warnings...),
	}

	var shotAt time.Time
	var shotSource string
	confidence := 0.35
	if parsed, ok := firstParsedTime(probe.CreationTimes); ok {
		shotAt = parsed
		shotSource = "ffprobe_metadata"
		confidence = 0.88
	} else if parsed, source, ok := timeFromFilename(originalName); ok {
		shotAt = parsed
		shotSource = source
		confidence = 0.72
	} else if statErr == nil {
		shotAt = info.ModTime()
		shotSource = "filesystem_modified"
		confidence = 0.45
	} else {
		item.Warnings = append(item.Warnings, fmt.Sprintf("could not stat file: %v", statErr))
	}

	if !shotAt.IsZero() {
		item.ShotAt = shotAt.Format(time.RFC3339)
		item.ShotAtSource = shotSource
	}

	item.Tags = BuildTags(originalName, shotAt, probe)
	item.NameParts = buildNameParts(shotAt, trip, item.Tags, sequence)
	item.RecommendedFileName = buildFileName(item.NameParts, ext)
	item.FinalFileName = item.RecommendedFileName
	item.Confidence = round(confidence, 2)
	return item
}

func BuildTags(fileName string, shotAt time.Time, probe ProbeResult) []string {
	seen := map[string]bool{}
	var tags []string
	add := func(tag string) {
		tag = slugify(tag)
		if tag == "" || seen[tag] {
			return
		}
		seen[tag] = true
		tags = append(tags, tag)
	}

	add("video")
	if strings.HasPrefix(strings.ToLower(fileName), "dji") || containsTagValue(probe.Tags, "dji") {
		add("dji")
	}
	if containsTagValue(probe.Tags, "osmo") {
		add("osmo")
	}
	if containsTagValue(probe.Tags, "pocket") {
		add("pocket")
	}
	if probe.Video != nil {
		add(probe.Video.Codec)
		add(resolutionTag(probe.Video.Width, probe.Video.Height))
		add(orientationTag(probe.Video.Width, probe.Video.Height))
		if fpsTag := frameRateTag(probe.Video.FPS); fpsTag != "" {
			add(fpsTag)
		}
		if probe.Video.FPS >= 100 {
			add("slow_motion")
		}
	}
	if probe.Audio != nil {
		add("audio")
	} else {
		add("silent")
	}
	if probe.Location != nil {
		for _, tag := range locationTags(probe.Location) {
			add(tag)
		}
	}
	if durationTag := durationTag(probe.DurationSeconds); durationTag != "" {
		add(durationTag)
	}
	if !shotAt.IsZero() {
		add(timeOfDayTag(shotAt))
	}

	sort.SliceStable(tags, func(i, j int) bool {
		return tagPriority(tags[i]) < tagPriority(tags[j])
	})
	return tags
}

func firstParsedTime(values []string) (time.Time, bool) {
	for _, value := range values {
		if parsed, ok := parseFlexibleTime(value); ok {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func timeFromFilename(fileName string) (time.Time, string, bool) {
	base := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	for _, pattern := range filenameDatePatterns {
		matches := pattern.FindStringSubmatch(base)
		if len(matches) == 7 {
			value := strings.Join(matches[1:7], "")
			parsed, err := time.ParseInLocation("20060102150405", value, time.Local)
			if err == nil {
				return parsed, "filename_datetime", true
			}
		}
		if len(matches) == 4 {
			value := strings.Join(matches[1:4], "")
			parsed, err := time.ParseInLocation("20060102", value, time.Local)
			if err == nil {
				return parsed, "filename_date", true
			}
		}
	}
	return time.Time{}, "", false
}

func parseFlexibleTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000000Z",
		"2006-01-02T15:04:05-0700",
		"2006-01-02T15:04:05.000-0700",
		"2006:01:02 15:04:05",
		"2006-01-02 15:04:05",
		"20060102150405",
		"20060102_150405",
		"20060102",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, true
		}
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func containsTagValue(tags map[string]string, needle string) bool {
	needle = strings.ToLower(needle)
	for key, value := range tags {
		if strings.Contains(strings.ToLower(key), needle) || strings.Contains(strings.ToLower(value), needle) {
			return true
		}
	}
	return false
}

func resolutionTag(width, height int) string {
	longSide := max(width, height)
	shortSide := min(width, height)
	switch {
	case longSide >= 7680:
		return "8k"
	case longSide >= 5120:
		return "5k"
	case longSide >= 3840 || shortSide >= 2160:
		return "4k"
	case longSide >= 2704:
		return "2_7k"
	case shortSide >= 1440:
		return "1440p"
	case shortSide >= 1080:
		return "1080p"
	case shortSide >= 720:
		return "720p"
	default:
		return ""
	}
}

func orientationTag(width, height int) string {
	switch {
	case width == 0 || height == 0:
		return ""
	case width > height:
		return "landscape"
	case height > width:
		return "vertical"
	default:
		return "square"
	}
}

func frameRateTag(fps float64) string {
	if fps <= 0 {
		return ""
	}
	rounded := int(math.Round(fps))
	return fmt.Sprintf("%dfps", rounded)
}

func durationTag(seconds float64) string {
	switch {
	case seconds <= 0:
		return ""
	case seconds < 15:
		return "short_clip"
	case seconds < 60:
		return "clip"
	case seconds < 300:
		return "long_take"
	default:
		return "extended_take"
	}
}

func timeOfDayTag(t time.Time) string {
	hour := t.Local().Hour()
	switch {
	case hour < 5:
		return "late_night"
	case hour < 11:
		return "morning"
	case hour < 16:
		return "afternoon"
	case hour < 20:
		return "evening"
	default:
		return "night"
	}
}

func locationTags(location *LocationInfo) []string {
	if location == nil {
		return nil
	}
	var tags []string
	if location.Latitude != 0 || location.Longitude != 0 {
		tags = append(tags, "gps")
		tags = append(tags, fmt.Sprintf("geo_%.4f_%.4f", location.Latitude, location.Longitude))
	}
	if location.Label != "" {
		tags = append(tags, location.Label)
	}
	if location.Notes != "" && location.Confidence >= 0.55 {
		tags = append(tags, location.Notes)
	}
	return tags
}

func contentTags(content *ContentInfo) []string {
	if content == nil {
		return nil
	}
	tags := append([]string{}, content.Tags...)
	tags = append(tags, content.AudioTags...)
	if content.AudioTranscript != "" {
		tags = append(tags, "speech")
	}
	if content.LocationGuess != "" && content.LocationConfidence >= 0.45 {
		tags = append(tags, content.LocationGuess)
	}
	return tags
}

func buildNameParts(shotAt time.Time, trip string, tags []string, sequence int) NameParts {
	parts := NameParts{
		Trip:     slugify(trip),
		Slug:     nameSlug(tags),
		Sequence: fmt.Sprintf("%03d", sequence),
	}
	if !shotAt.IsZero() {
		local := shotAt.Local()
		parts.Date = local.Format("20060102")
		parts.Time = local.Format("150405")
	}
	return parts
}

func nameSlug(tags []string) string {
	preferred := []string{}
	for _, tag := range tags {
		switch tag {
		case "video", "audio", "silent", "clip":
			continue
		default:
			preferred = append(preferred, tag)
		}
		if len(preferred) == 4 {
			break
		}
	}
	if len(preferred) == 0 {
		return "clip"
	}
	return strings.Join(preferred, "_")
}

func buildFileName(parts NameParts, ext string) string {
	if ext == "" {
		ext = ".mp4"
	}
	segments := []string{}
	if parts.Date != "" {
		segments = append(segments, parts.Date)
	}
	if parts.Time != "" {
		segments = append(segments, parts.Time)
	}
	if parts.Trip != "" {
		segments = append(segments, parts.Trip)
	}
	if parts.Slug != "" {
		segments = append(segments, parts.Slug)
	}
	segments = append(segments, parts.Sequence)
	return strings.Join(segments, "_") + strings.ToLower(ext)
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	previousUnderscore := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			builder.WriteRune(unicode.ToLower(r))
			previousUnderscore = false
			continue
		}
		if !previousUnderscore {
			builder.WriteRune('_')
			previousUnderscore = true
		}
	}
	return strings.Trim(builder.String(), "_")
}

func tagPriority(tag string) int {
	switch tag {
	case "video":
		return 0
	case "dji", "osmo", "pocket":
		return 10
	case "4k", "5k", "8k", "2_7k", "1440p", "1080p", "720p":
		return 20
	case "landscape", "vertical", "square":
		return 30
	case "24fps", "25fps", "30fps", "50fps", "60fps", "120fps":
		return 40
	case "morning", "afternoon", "evening", "night", "late_night":
		return 50
	default:
		return 100
	}
}

func round(value float64, places int) float64 {
	if value == 0 {
		return 0
	}
	factor := math.Pow10(places)
	return math.Round(value*factor) / factor
}
