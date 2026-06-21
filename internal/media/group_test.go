package media

import "testing"

func TestGroupForItemUsesContentSummary(t *testing.T) {
	item := Item{
		Content: &ContentInfo{
			SceneSummary: "Traveler buying a ticket at a train station vending machine",
		},
	}

	group := groupForItem(item)

	if group.Key != "train" {
		t.Fatalf("expected train group, got %#v", group)
	}
}

func TestGroupForItemUsesKoreanTags(t *testing.T) {
	item := Item{
		Tags: []string{"여행", "카페"},
	}

	group := groupForItem(item)

	if group.Key != "food" {
		t.Fatalf("expected food group, got %#v", group)
	}
}
