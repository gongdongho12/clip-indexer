package media

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var iso6709Pattern = regexp.MustCompile(`([+-]\d+(?:\.\d+)?)([+-]\d+(?:\.\d+)?)([+-]\d+(?:\.\d+)?)?/??`)

type ffprobeOutput struct {
	Format  ffprobeFormat   `json:"format"`
	Streams []ffprobeStream `json:"streams"`
}

type ffprobeFormat struct {
	FormatName string            `json:"format_name"`
	Duration   string            `json:"duration"`
	Tags       map[string]string `json:"tags"`
}

type ffprobeStream struct {
	CodecType    string            `json:"codec_type"`
	CodecName    string            `json:"codec_name"`
	Width        int               `json:"width"`
	Height       int               `json:"height"`
	AvgFrameRate string            `json:"avg_frame_rate"`
	RFrameRate   string            `json:"r_frame_rate"`
	Duration     string            `json:"duration"`
	SampleRate   string            `json:"sample_rate"`
	Channels     int               `json:"channels"`
	Tags         map[string]string `json:"tags"`
}

func Probe(ctx context.Context, ffprobePath string, path string) ProbeResult {
	result := ProbeResult{Tags: map[string]string{}}
	cmd := exec.CommandContext(
		ctx,
		ffprobePath,
		"-v", "error",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		path,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("ffprobe failed: %v", err))
		if text := strings.TrimSpace(string(output)); text != "" {
			result.Warnings = append(result.Warnings, text)
		}
		return result
	}

	var raw ffprobeOutput
	if err := json.Unmarshal(output, &raw); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("could not parse ffprobe JSON: %v", err))
		return result
	}

	result.FormatName = raw.Format.FormatName
	result.DurationSeconds = parseFloat(raw.Format.Duration)
	mergeTags(result.Tags, raw.Format.Tags)
	result.CreationTimes = append(result.CreationTimes, dateLikeTags(raw.Format.Tags)...)

	for _, stream := range raw.Streams {
		mergeTags(result.Tags, stream.Tags)
		result.CreationTimes = append(result.CreationTimes, dateLikeTags(stream.Tags)...)
		switch stream.CodecType {
		case "video":
			if result.Video == nil {
				result.Video = &VideoInfo{
					Codec:  stream.CodecName,
					Width:  stream.Width,
					Height: stream.Height,
					FPS:    firstPositiveFPS(stream.AvgFrameRate, stream.RFrameRate),
				}
			}
			if result.DurationSeconds == 0 {
				result.DurationSeconds = parseFloat(stream.Duration)
			}
		case "audio":
			if result.Audio == nil {
				result.Audio = &AudioInfo{
					Codec:      stream.CodecName,
					Channels:   stream.Channels,
					SampleRate: parseInt(stream.SampleRate),
				}
			}
		}
	}

	result.Location = extractLocation(result.Tags)
	return result
}

func mergeTags(target map[string]string, source map[string]string) {
	for key, value := range source {
		if strings.TrimSpace(value) == "" {
			continue
		}
		target[key] = value
	}
}

func dateLikeTags(tags map[string]string) []string {
	var values []string
	for key, value := range tags {
		normalized := strings.ToLower(key)
		if strings.Contains(normalized, "creation") ||
			strings.Contains(normalized, "date") ||
			strings.Contains(normalized, "time") {
			values = append(values, value)
		}
	}
	return values
}

func extractLocation(tags map[string]string) *LocationInfo {
	if len(tags) == 0 {
		return nil
	}

	for key, value := range tags {
		normalized := strings.ToLower(key)
		if strings.Contains(normalized, "location") || strings.Contains(normalized, "iso6709") {
			if location := parseISO6709(value); location != nil {
				location.Source = key
				location.Confidence = 0.92
				return location
			}
		}
	}

	latValue, latKey := findTag(tags, "gpslatitude", "latitude")
	lonValue, lonKey := findTag(tags, "gpslongitude", "longitude")
	if latValue == "" || lonValue == "" {
		return nil
	}
	lat, okLat := parseCoordinate(latValue)
	lon, okLon := parseCoordinate(lonValue)
	if !okLat || !okLon {
		return nil
	}
	if ref := tagValue(tags, "gpslatituderef", "latituderef"); strings.EqualFold(ref, "S") {
		lat = -abs(lat)
	}
	if ref := tagValue(tags, "gpslongituderef", "longituderef"); strings.EqualFold(ref, "W") {
		lon = -abs(lon)
	}
	location := &LocationInfo{
		Latitude:   lat,
		Longitude:  lon,
		Source:     latKey + "," + lonKey,
		Confidence: 0.9,
	}
	if altitude := tagValue(tags, "gpsaltitude", "altitude"); altitude != "" {
		if parsed, ok := parseCoordinate(altitude); ok {
			location.AltitudeMeters = parsed
		}
	}
	return location
}

func parseISO6709(value string) *LocationInfo {
	value = strings.TrimSpace(value)
	matches := iso6709Pattern.FindStringSubmatch(value)
	if len(matches) < 3 {
		return nil
	}
	lat := parseFloat(matches[1])
	lon := parseFloat(matches[2])
	if lat == 0 && lon == 0 {
		return nil
	}
	location := &LocationInfo{
		Latitude:  lat,
		Longitude: lon,
	}
	if len(matches) > 3 && matches[3] != "" {
		location.AltitudeMeters = parseFloat(matches[3])
	}
	return location
}

func parseCoordinate(value string) (float64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	value = strings.TrimSuffix(value, "m")
	value = strings.TrimSpace(value)
	if parsed, err := strconv.ParseFloat(value, 64); err == nil {
		return parsed, true
	}

	replacer := strings.NewReplacer("deg", " ", "°", " ", "'", " ", "\"", " ", ",", " ")
	parts := strings.Fields(replacer.Replace(value))
	if len(parts) == 0 {
		return 0, false
	}
	degrees, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, false
	}
	minutes := 0.0
	seconds := 0.0
	if len(parts) > 1 {
		minutes, _ = strconv.ParseFloat(parts[1], 64)
	}
	if len(parts) > 2 {
		seconds, _ = strconv.ParseFloat(parts[2], 64)
	}
	sign := 1.0
	if degrees < 0 {
		sign = -1
		degrees = -degrees
	}
	return sign * (degrees + minutes/60 + seconds/3600), true
}

func findTag(tags map[string]string, names ...string) (string, string) {
	for key, value := range tags {
		normalized := normalizeTagKey(key)
		for _, name := range names {
			if normalized == normalizeTagKey(name) {
				return value, key
			}
		}
	}
	return "", ""
}

func tagValue(tags map[string]string, names ...string) string {
	value, _ := findTag(tags, names...)
	return strings.TrimSpace(value)
}

func normalizeTagKey(value string) string {
	value = strings.ToLower(value)
	var builder strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func abs(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func parseFloat(value string) float64 {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0
	}
	return parsed
}

func parseInt(value string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return parsed
}

func firstPositiveFPS(values ...string) float64 {
	for _, value := range values {
		fps := parseFPS(value)
		if fps > 0 {
			return fps
		}
	}
	return 0
}

func parseFPS(value string) float64 {
	value = strings.TrimSpace(value)
	if value == "" || value == "0/0" {
		return 0
	}
	parts := strings.Split(value, "/")
	if len(parts) == 1 {
		return parseFloat(parts[0])
	}
	numerator := parseFloat(parts[0])
	denominator := parseFloat(parts[1])
	if denominator == 0 {
		return 0
	}
	return numerator / denominator
}
