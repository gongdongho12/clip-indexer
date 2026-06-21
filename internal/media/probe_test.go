package media

import "testing"

func TestParseFPS(t *testing.T) {
	if got := parseFPS("60000/1001"); got < 59.9 || got > 60 {
		t.Fatalf("expected about 59.94, got %f", got)
	}
	if got := parseFPS("0/0"); got != 0 {
		t.Fatalf("expected zero fps, got %f", got)
	}
}

func TestExtractLocationFromISO6709(t *testing.T) {
	location := extractLocation(map[string]string{
		"com.apple.quicktime.location.ISO6709": "+37.566500+126.978000+012.300/",
	})
	if location == nil {
		t.Fatal("expected location")
	}
	if location.Latitude != 37.5665 {
		t.Fatalf("unexpected latitude: %f", location.Latitude)
	}
	if location.Longitude != 126.978 {
		t.Fatalf("unexpected longitude: %f", location.Longitude)
	}
	if location.AltitudeMeters != 12.3 {
		t.Fatalf("unexpected altitude: %f", location.AltitudeMeters)
	}
}

func TestExtractLocationFromLatitudeLongitudeTags(t *testing.T) {
	location := extractLocation(map[string]string{
		"GPS Latitude":      "37 deg 33 59.40",
		"GPS Latitude Ref":  "N",
		"GPS Longitude":     "126 deg 58 40.80",
		"GPS Longitude Ref": "E",
	})
	if location == nil {
		t.Fatal("expected location")
	}
	if location.Latitude < 37.566 || location.Latitude > 37.567 {
		t.Fatalf("unexpected latitude: %f", location.Latitude)
	}
	if location.Longitude < 126.977 || location.Longitude > 126.979 {
		t.Fatalf("unexpected longitude: %f", location.Longitude)
	}
}
