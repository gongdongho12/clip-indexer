package media

import (
	"slices"
	"testing"
	"time"
)

func TestTimeFromFilenameDJI(t *testing.T) {
	parsed, source, ok := timeFromFilename("DJI_20240615123012_0001_D.MP4")
	if !ok {
		t.Fatal("expected filename datetime")
	}
	if source != "filename_datetime" {
		t.Fatalf("unexpected source: %s", source)
	}
	if got := parsed.Format("20060102150405"); got != "20240615123012" {
		t.Fatalf("unexpected time: %s", got)
	}
}

func TestBuildTagsForVerticalDJI4K(t *testing.T) {
	tags := BuildTags("DJI_20240615123012_0001_D.MP4", time.Date(2024, 6, 15, 18, 30, 0, 0, time.Local), ProbeResult{
		DurationSeconds: 12,
		Tags:            map[string]string{"model": "DJI Osmo Pocket 3"},
		Video: &VideoInfo{
			Codec:  "h264",
			Width:  2160,
			Height: 3840,
			FPS:    59.94,
		},
		Audio: &AudioInfo{Codec: "aac", Channels: 2, SampleRate: 48000},
	})
	for _, expected := range []string{"video", "dji", "osmo", "pocket", "4k", "vertical", "60fps", "evening", "short_clip"} {
		if !slices.Contains(tags, expected) {
			t.Fatalf("expected tag %q in %v", expected, tags)
		}
	}
}

func TestBuildItemNameIncludesTrip(t *testing.T) {
	item := BuildItem("/tmp/DJI_20240615123012_0001_D.MP4", 7, "Seoul Spring", ProbeResult{
		Tags: map[string]string{"model": "DJI"},
		Video: &VideoInfo{
			Codec:  "h265",
			Width:  3840,
			Height: 2160,
			FPS:    29.97,
		},
	})
	if item.NameParts.Trip != "seoul_spring" {
		t.Fatalf("unexpected trip slug: %s", item.NameParts.Trip)
	}
	if item.FinalFileName != "20240615_123012_seoul_spring_dji_4k_landscape_30fps_007.mp4" {
		t.Fatalf("unexpected final file name: %s", item.FinalFileName)
	}
}

func TestBuildItemNamePreservesKoreanTrip(t *testing.T) {
	item := BuildItem("/tmp/DJI_20240615123012_0001_D.MP4", 7, "서울 여행", ProbeResult{
		Tags: map[string]string{"model": "DJI"},
		Video: &VideoInfo{
			Codec:  "h265",
			Width:  3840,
			Height: 2160,
			FPS:    29.97,
		},
	})
	if item.NameParts.Trip != "서울_여행" {
		t.Fatalf("unexpected Korean trip slug: %s", item.NameParts.Trip)
	}
	if item.FinalFileName != "20240615_123012_서울_여행_dji_4k_landscape_30fps_007.mp4" {
		t.Fatalf("unexpected final file name: %s", item.FinalFileName)
	}
}

func TestSlugifyPreservesKoreanTags(t *testing.T) {
	if got := slugify("서울 맛집 / 야경"); got != "서울_맛집_야경" {
		t.Fatalf("unexpected slug: %s", got)
	}
}
