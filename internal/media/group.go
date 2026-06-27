package media

import (
	"fmt"
	"strings"
	"unicode"
)

type groupRule struct {
	keywords map[string]float64
	info     GroupInfo
}

var groupRules = []groupRule{
	{
		keywords: map[string]float64{
			"airport": 2.0, "terminal": 2.0, "flight": 2.0, "airplane": 2.0, "kansai_airport": 3.0, "kix": 3.0,
			"공항": 2.0, "비행기": 2.0, "터미널": 2.0,
		},
		info: GroupInfo{Key: "airport", Label: "Airport", Folder: "airport"},
	},
	{
		keywords: map[string]float64{
			"train": 2.5, "station": 1.5, "subway": 2.5, "metro": 2.5, "rail": 2.0, "haruka": 3.0,
			"ticket_machine": 2.0, "transportation": 1.0, "ticket vending": 2.0,
			"기차": 2.5, "열차": 2.5, "역": 1.5, "지하철": 2.5, "전철": 2.5,
		},
		info: GroupInfo{Key: "train", Label: "Train", Folder: "train"},
	},
	{
		keywords: map[string]float64{
			"bus": 2.5, "taxi": 2.5, "car": 1.5, "driving": 2.0, "ferry": 2.5, "boat": 2.5, "transport": 1.0,
			"버스": 2.5, "택시": 2.5, "자동차": 1.5, "운전": 2.0, "배": 2.5,
		},
		info: GroupInfo{Key: "transport", Label: "Transport", Folder: "transport"},
	},
	{
		keywords: map[string]float64{
			"restaurant": 2.5, "cafe": 2.5, "food": 2.0, "meal": 2.0, "breakfast": 2.0, "lunch": 2.0, "dinner": 2.0,
			"ramen": 3.0, "sushi": 3.0, "coffee": 2.0, "bar": 1.5,
			"식당": 2.5, "카페": 2.5, "음식": 2.0, "밥": 2.0, "라멘": 3.0, "초밥": 3.0, "커피": 2.0,
		},
		info: GroupInfo{Key: "food", Label: "Food", Folder: "food"},
	},
	{
		keywords: map[string]float64{
			"hotel": 2.5, "room": 1.5, "lobby": 2.0, "accommodation": 2.0, "checkin": 2.5, "check-in": 2.5,
			"호텔": 2.5, "숙소": 2.0, "객실": 1.5, "로비": 2.0,
		},
		info: GroupInfo{Key: "hotel", Label: "Hotel", Folder: "hotel"},
	},
	{
		keywords: map[string]float64{
			"temple": 2.5, "shrine": 2.5, "castle": 2.5, "bridge": 1.5, "landmark": 2.0,
			"observatory": 2.5, "museum": 2.5, "tower": 2.0, "akashi_kaikyo_bridge": 3.0,
			"사찰": 2.5, "절": 2.5, "신사": 2.5, "성": 2.5, "다리": 1.5, "박물관": 2.5, "전망대": 2.5,
		},
		info: GroupInfo{Key: "landmark", Label: "Landmark", Folder: "landmark"},
	},
	{
		keywords: map[string]float64{
			"beach": 2.0, "sea": 1.5, "ocean": 1.5, "mountain": 2.0, "park": 1.5, "forest": 1.5,
			"coastal": 2.0, "lake": 2.0, "river": 1.5, "sunset": 1.5, "scenic": 1.5, "view": 1.0,
			"바다": 1.5, "해변": 2.0, "산": 2.0, "공원": 1.5, "숲": 1.5, "강": 1.5, "노을": 1.5, "풍경": 1.5,
		},
		info: GroupInfo{Key: "nature", Label: "Nature", Folder: "nature"},
	},
	{
		keywords: map[string]float64{
			"shop": 2.0, "shopping": 2.5, "market": 2.0, "store": 2.0, "mall": 2.5, "convenience": 2.0,
			"쇼핑": 2.5, "시장": 2.0, "상점": 2.0, "편의점": 2.0, "마트": 2.0,
		},
		info: GroupInfo{Key: "shopping", Label: "Shopping", Folder: "shopping"},
	},
	{
		keywords: map[string]float64{
			"street": 1.5, "city": 1.5, "downtown": 2.0, "night_view": 2.0, "walking": 1.0,
			"neighborhood": 1.5, "urban": 1.5,
			"거리": 1.5, "도시": 1.5, "야경": 2.0, "산책": 1.0, "시내": 1.5,
		},
		info: GroupInfo{Key: "city", Label: "City", Folder: "city"},
	},
	{
		keywords: map[string]float64{
			"people": 1.5, "family": 2.0, "friend": 2.0, "portrait": 2.0, "selfie": 2.0,
			"사람": 1.5, "가족": 2.0, "친구": 2.0, "인물": 2.0, "셀피": 2.0,
		},
		info: GroupInfo{Key: "people", Label: "People", Folder: "people"},
	},
}

func updateItemGroup(item *Item) {
	group := groupForItem(*item)
	item.Group = &group
}

func updateItemGroups(items []Item) {
	for index := range items {
		updateItemGroup(&items[index])
	}
}

func groupForItem(item Item) GroupInfo {
	words := tokenize(strings.Join(groupTextParts(item), " "))

	// Create a map of word frequencies or presence for quick lookups
	wordSet := make(map[string]int)
	for _, w := range words {
		wordSet[w]++
	}

	bestScore := 0.0
	var bestInfo GroupInfo
	matchedReason := "fallback"
	foundAny := false

	for _, rule := range groupRules {
		score := 0.0
		reasonWord := ""
		for keyword, weight := range rule.keywords {
			// Check if keyword has spaces (multi-word phrase)
			if strings.Contains(keyword, " ") {
				joinedText := strings.ToLower(strings.Join(words, " "))
				if strings.Contains(joinedText, keyword) {
					score += weight
					if reasonWord == "" {
						reasonWord = keyword
					}
				}
			} else {
				// Single word check
				if count, found := wordSet[keyword]; found {
					score += weight * float64(count)
					if reasonWord == "" {
						reasonWord = keyword
					}
				}
			}
		}

		if score > bestScore {
			bestScore = score
			bestInfo = rule.info
			matchedReason = reasonWord
			foundAny = true
		}
	}

	if foundAny {
		bestInfo.Reason = fmt.Sprintf("%s (score: %.1f)", matchedReason, bestScore)
		return bestInfo
	}

	return GroupInfo{Key: "other", Label: "Other", Folder: "other", Reason: "fallback"}
}

func tokenize(text string) []string {
	var words []string
	var current strings.Builder
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
		}
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}
	return words
}

func groupTextParts(item Item) []string {
	parts := append([]string{}, item.Tags...)
	if item.Location != nil {
		parts = append(parts, item.Location.Label, item.Location.Notes)
	}
	if item.Content != nil {
		parts = append(parts,
			item.Content.SceneSummary,
			item.Content.AudioSummary,
			item.Content.AudioTranscript,
			item.Content.LocationGuess,
			item.Content.Notes,
		)
		parts = append(parts, item.Content.Tags...)
		parts = append(parts, item.Content.AudioTags...)
	}
	return parts
}
