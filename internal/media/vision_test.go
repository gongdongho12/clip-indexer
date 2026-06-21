package media

import "testing"

func TestSampleSeconds(t *testing.T) {
	got := sampleSeconds(9, 2)
	if len(got) != 2 {
		t.Fatalf("expected two samples, got %v", got)
	}
	if got[0] != 3 || got[1] != 6 {
		t.Fatalf("unexpected samples: %v", got)
	}
}

func TestClamp01(t *testing.T) {
	if clamp01(-1) != 0 {
		t.Fatal("negative should clamp to zero")
	}
	if clamp01(2) != 1 {
		t.Fatal("above one should clamp to one")
	}
	if clamp01(0.4) != 0.4 {
		t.Fatal("in-range value should stay unchanged")
	}
}
